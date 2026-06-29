package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"credential-priority/internal/apply"
	"credential-priority/internal/config"
	"credential-priority/internal/core"
	"credential-priority/internal/host"
	"credential-priority/internal/priority"
	"credential-priority/internal/schedule"
	"credential-priority/internal/state"
)

var errMissingHostCallbacks = errors.New("runtime: host callbacks are required")

const (
	autoQuotaProbeAttempts = 3
	autoQuotaProbeDelay    = 10 * time.Second
)

func (r *Runtime) runProductionTask(ctx context.Context, request TaskRequest) error {
	if r.hostCallbacks == nil {
		return errMissingHostCallbacks
	}
	now := r.clock.Now().UTC()
	client := host.NewClient(r.hostCallbacks)
	files, err := client.ListAuthFiles(ctx)
	if err != nil {
		return err
	}
	credentials, accountIDs := credentialsFromAuthFiles(files)
	credentials = filterCredentialsByProvider(credentials, request.Config)
	credentials = filterCredentialsByAuthIndex(credentials, request.AuthIndexes)
	credentials, authMaterials, err := enrichCredentialsFromAuthDocuments(ctx, client, credentials)
	if err != nil {
		return err
	}
	store, err := state.Load(ctx, request.Config.CachePath)
	if err != nil {
		return err
	}
	probes, err := probesForRequest(credentials, scheduleOptions(request.Config, now), request.AuthIndexes)
	if err != nil {
		return err
	}
	evidence, err := r.collectEvidenceForTrigger(ctx, collectInput{client: client, store: store, probes: probes, accountIDs: accountIDs, authMaterials: authMaterials, now: now, cacheTTL: request.Config.CacheTTL, forceProbe: request.Trigger == TriggerManualApply, antigravityModelGroup: request.Config.AntigravityModelGroup}, request.Trigger)
	if err != nil {
		return err
	}
	if err := store.SaveAtomic(ctx); err != nil {
		return err
	}
	plan := priority.PlanFreshOnly(credentials, evidence, priorityOptions(request.Config, now))
	if request.Trigger == TriggerManual {
		result := apply.Result{Snapshot: apply.Snapshot(plan)}
		r.snapshotRun(result, "dry-run plan generated")
		return nil
	}
	if request.Trigger == TriggerManualApply {
		failures := manualProbeFailures(plan, evidence)
		result, err := apply.Apply(ctx, apply.Request{Host: client, Auditor: r, Plan: plan, ReportSkippedPlan: true})
		if err != nil {
			return err
		}
		if len(failures) > 0 {
			appendManualProbeFailures(&result, failures)
			r.snapshotRun(result, fmt.Sprintf("apply attempted=%d succeeded=%d failed=%d skipped=%d", result.Attempted, result.Succeeded, result.Failed, result.Skipped))
			return nil
		}
		r.snapshotRun(result, fmt.Sprintf("apply attempted=%d succeeded=%d failed=%d skipped=%d", result.Attempted, result.Succeeded, result.Failed, result.Skipped))
		return nil
	}
	result, err := apply.Apply(ctx, apply.Request{Host: client, Auditor: r, Plan: plan, ReportSkippedPlan: true})
	if err != nil {
		return err
	}
	r.snapshotRun(result, fmt.Sprintf("apply attempted=%d succeeded=%d failed=%d skipped=%d", result.Attempted, result.Succeeded, result.Failed, result.Skipped))
	return nil
}

func (r *Runtime) collectEvidenceForTrigger(ctx context.Context, input collectInput, trigger Trigger) ([]priority.ProbeEvidence, error) {
	if trigger != TriggerAutoApply {
		return collectFreshEvidence(ctx, input)
	}
	var evidence []priority.ProbeEvidence
	for attempt := 1; attempt <= autoQuotaProbeAttempts; attempt++ {
		current, err := collectFreshEvidence(ctx, input)
		if err != nil {
			return nil, err
		}
		evidence = current
		if !hasProbeFailure(current) || attempt == autoQuotaProbeAttempts {
			return evidence, nil
		}
		input.forceProbe = true
		if err := r.sleeper.Sleep(ctx, autoQuotaProbeDelay); err != nil {
			return nil, err
		}
	}
	return evidence, nil
}

func hasProbeFailure(evidence []priority.ProbeEvidence) bool {
	return slices.ContainsFunc(evidence, func(item priority.ProbeEvidence) bool {
		return item.Status == priority.EvidenceStatusProbeFailed
	})
}

func manualProbeFailures(plan priority.Plan, evidence []priority.ProbeEvidence) []apply.ChangeResult {
	failures := make(map[string]priority.ProbeEvidence)
	for _, item := range evidence {
		if item.Status == priority.EvidenceStatusProbeFailed {
			failures[item.AuthIndex] = item
		}
	}
	if len(failures) == 0 {
		return nil
	}
	results := make([]apply.ChangeResult, 0, len(failures))
	for _, item := range plan.Items {
		failure, ok := failures[item.Credential.AuthIndex]
		if !ok {
			continue
		}
		failedItem := item
		failedItem.Credential.Provider = failure.Provider
		failedItem.Reason = "failedQuotaFetch"
		change := apply.FailureResult(failedItem, fmt.Errorf("failedQuotaFetch"))
		results = append(results, change)
	}
	return results
}

func appendManualProbeFailures(result *apply.Result, failures []apply.ChangeResult) {
	for _, failure := range failures {
		replaceOrAppendManualProbeFailure(result, failure)
	}
}

func replaceOrAppendManualProbeFailure(result *apply.Result, failure apply.ChangeResult) {
	key := manualProbeFailureKey(failure)
	for index := 0; index < len(result.Changes); index++ {
		if manualProbeFailureKey(result.Changes[index]) != key {
			continue
		}
		decrementResultCounter(result, result.Changes[index])
		result.Changes = append(result.Changes[:index], result.Changes[index+1:]...)
		index--
	}
	result.Changes = append(result.Changes, failure)
	result.Attempted++
	result.Failed++
}

func manualProbeFailureKey(change apply.ChangeResult) string {
	if change.RetryAuthIndex != "" {
		return change.RetryAuthIndex
	}
	return change.AuthIndex
}

func decrementResultCounter(result *apply.Result, change apply.ChangeResult) {
	switch change.Status {
	case apply.ChangeStatusSuccess:
		result.Attempted--
		result.Succeeded--
	case apply.ChangeStatusFailed:
		result.Attempted--
		result.Failed--
	case apply.ChangeStatusSkipped:
		result.Skipped--
	}
}

func probesForRequest(credentials []core.Credential, options schedule.Options, authIndexes []string) ([]schedule.Probe, error) {
	if len(authIndexes) == 0 {
		probePlan, err := schedule.PlanProbeSchedule(credentials, options)
		if err != nil {
			return nil, err
		}
		return probePlan.Immediate, nil
	}
	return probesAtCurrentTime(credentials, options.Clock.Now()), nil
}

func filterCredentialsByAuthIndex(credentials []core.Credential, authIndexes []string) []core.Credential {
	if len(authIndexes) == 0 {
		return credentials
	}
	allowed := make(map[string]struct{}, len(authIndexes))
	for _, authIndex := range authIndexes {
		allowed[authIndex] = struct{}{}
	}
	filtered := make([]core.Credential, 0, len(credentials))
	for _, credential := range credentials {
		if _, ok := allowed[credential.AuthIndex]; ok {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

func probesAtCurrentTime(credentials []core.Credential, now time.Time) []schedule.Probe {
	probes := make([]schedule.Probe, len(credentials))
	for index, credential := range credentials {
		probes[index] = schedule.Probe{Credential: credential, NextProbeAt: now}
	}
	return probes
}

func filterCredentialsByProvider(credentials []core.Credential, cfg config.Config) []core.Credential {
	if cfg.ProviderScope != config.ProviderScopeSelected || len(cfg.SelectedProviders) == 0 {
		filtered := make([]core.Credential, 0, len(credentials))
		for _, credential := range credentials {
			p := filterProvider(credential)
			if p == core.ProviderAntigravity || p == core.ProviderCodex {
				filtered = append(filtered, credential)
			}
		}
		return filtered
	}
	selected := make(map[core.Provider]struct{}, len(cfg.SelectedProviders))
	for _, provider := range cfg.SelectedProviders {
		selected[core.Provider(provider)] = struct{}{}
	}
	filtered := make([]core.Credential, 0, len(credentials))
	for _, credential := range credentials {
		if _, ok := selected[filterProvider(credential)]; ok {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

func filterProvider(credential core.Credential) core.Provider {
	if credential.Provider != "" {
		return credential.Provider
	}
	switch credential.Type {
	case core.CredentialTypeCodex:
		return core.ProviderCodex
	case core.CredentialTypeAntigravity:
		return core.ProviderAntigravity
	default:
		return core.ProviderUnknown
	}
}

func credentialsFromAuthFiles(files []host.AuthFile) ([]core.Credential, map[string]string) {
	credentials := make([]core.Credential, len(files))
	accountIDs := make(map[string]string, len(files))
	for index, file := range files {
		credentials[index] = core.Credential{Name: file.Name, AuthIndex: file.AuthIndex, Provider: core.Provider(file.Provider), Type: core.CredentialType(file.Type), Status: core.CredentialStatus(file.Status), Disabled: file.Disabled, Unavailable: file.Unavailable, Priority: file.Priority, PriorityMissing: file.PriorityMissing, Account: file.Account, Email: file.Email, PlanType: core.PlanType(file.IDToken.PlanType), RawJSON: append([]byte(nil), file.RawJSON...)}
		accountIDs[file.AuthIndex] = file.IDToken.ChatGPTAccountID
	}
	return credentials, accountIDs
}

func scheduleOptions(cfg config.Config, now time.Time) schedule.Options {
	return schedule.Options{Clock: fixedClock{now: now}, RNG: zeroRNG{}, TopPriorityProbeCount: cfg.TopPriorityProbeCount, ActiveGroupSize: cfg.ActiveGroupSize, ActiveGroupJitter: cfg.ActiveGroupJitter, DisabledGroupSize: cfg.DisabledGroupSize, DisabledProbeInterval: cfg.DisabledProbeInterval}
}

func priorityOptions(cfg config.Config, now time.Time) priority.Options {
	options := priority.Options{Now: now, MaxPriority: 100, MinChange: cfg.MinChange, PaidFirst: true, ResetBoostWithin: 2 * time.Hour, ResetBoost: 50}
	if cfg.PriorityRules.Enabled {
		freeDepletedPriority := cfg.PriorityRules.Codex.FreeDepletedPriority
		freeDepletedDisabled := cfg.PriorityRules.Codex.FreeDepletedDisabled
		paidDepletedKeepsEnabled := cfg.PriorityRules.Codex.PaidDepletedKeepsEnabled
		options.StartPriorityByProvider = map[core.Provider]int{
			core.ProviderAntigravity: cfg.PriorityRules.Antigravity.StartPriority,
			core.ProviderCodex:       cfg.PriorityRules.Codex.StartPriority,
		}
		options.CodexFreeDepletedPriority = &freeDepletedPriority
		options.CodexFreeDepletedDisabled = &freeDepletedDisabled
		options.CodexPaidDepletedKeepsEnabled = &paidDepletedKeepsEnabled
	}
	return options
}

func probePolicy(cacheTTL time.Duration) state.ProbePolicy {
	return state.ProbePolicy{TTL: cacheTTL, ResetStaleAfter: time.Hour}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type zeroRNG struct{}

func (zeroRNG) Int63n(int64) int64 {
	return 0
}

func (r *Runtime) SaveSnapshot(ctx context.Context, snapshot apply.PlanSnapshot) error {
	return ctx.Err()
}

func (r *Runtime) RecordEvent(ctx context.Context, event apply.AuditEvent) error {
	return ctx.Err()
}

var _ apply.Auditor = (*Runtime)(nil)
