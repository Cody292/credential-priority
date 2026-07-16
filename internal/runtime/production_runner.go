package runtime

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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
	probes, err := probesForRequest(ctx, store, credentials, scheduleOptions(request.Config, now), request.AuthIndexes, request.Config.AntigravityModelGroup, request.Trigger)
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
	plan = withProbeFailureTemporaryDisables(plan, evidence)
	if request.Trigger == TriggerManual {
		result := apply.Result{Snapshot: apply.Snapshot(plan)}
		r.snapshotRun(result, "dry-run plan generated")
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

func withProbeFailureTemporaryDisables(plan priority.Plan, evidence []priority.ProbeEvidence) priority.Plan {
	disables := probeFailureDisableChanges(plan, evidence)
	if len(disables) == 0 {
		return plan
	}
	byAuth := make(map[string]struct{}, len(disables))
	for _, change := range disables {
		byAuth[change.Credential.AuthIndex] = struct{}{}
	}
	for index := range plan.Items {
		if _, ok := byAuth[plan.Items[index].Credential.AuthIndex]; !ok {
			continue
		}
		plan.Items[index].Disabled = true
		plan.Items[index].Reason = "failedQuotaFetch"
	}
	changeIndex := make(map[string]int, len(plan.Changes))
	for index, change := range plan.Changes {
		changeIndex[change.Credential.AuthIndex] = index
	}
	for _, change := range disables {
		if existing, ok := changeIndex[change.Credential.AuthIndex]; ok {
			plan.Changes[existing].Disabled = true
			plan.Changes[existing].EvidenceFresh = true
			if plan.Changes[existing].Reason == "" || plan.Changes[existing].Reason == "keep current state" {
				plan.Changes[existing].Reason = change.Reason
			}
			continue
		}
		plan.Changes = append(plan.Changes, change)
	}
	return plan
}

func probeFailureDisableChanges(plan priority.Plan, evidence []priority.ProbeEvidence) []priority.Change {
	failures := make(map[string]priority.ProbeEvidence)
	for _, item := range evidence {
		if item.Status == priority.EvidenceStatusProbeFailed {
			failures[item.AuthIndex] = item
		}
	}
	if len(failures) == 0 {
		return nil
	}
	changes := make([]priority.Change, 0, len(failures))
	for _, item := range plan.Items {
		failure, ok := failures[item.Credential.AuthIndex]
		if !ok {
			continue
		}
		if item.Credential.Disabled {
			continue
		}
		credential := item.Credential
		if failure.Provider != "" {
			credential.Provider = failure.Provider
		}
		changes = append(changes, priority.Change{
			Credential:    credential,
			Priority:      credential.Priority,
			Disabled:      true,
			EvidenceFresh: true,
			Reason:        "failedQuotaFetch",
		})
	}
	return changes
}

func probesForRequest(ctx context.Context, store *state.Store, credentials []core.Credential, options schedule.Options, authIndexes []string, modelGroup config.AntigravityModelGroup, trigger Trigger) ([]schedule.Probe, error) {
	if trigger == TriggerManual || trigger == TriggerManualApply {
		return probesAtCurrentTime(credentials, options.Clock.Now()), nil
	}
	if len(authIndexes) == 0 {
		probePlan, err := schedule.PlanProbeSchedule(credentials, options)
		if err != nil {
			return nil, err
		}
		return dueProbes(ctx, store, probePlan, options.Clock.Now(), modelGroup)
	}
	return probesAtCurrentTime(credentials, options.Clock.Now()), nil
}

func dueProbes(ctx context.Context, store *state.Store, plan schedule.Plan, now time.Time, modelGroup config.AntigravityModelGroup) ([]schedule.Probe, error) {
	result := make([]schedule.Probe, 0, len(plan.Immediate))
	for _, probe := range plan.Immediate {
		provider := filterProvider(probe.Credential)
		groupName := probeModelGroup(provider, modelGroup)
		needsProbe, err := store.NeedsProbe(ctx, state.ProbeCheck{AuthIndex: probe.Credential.AuthIndex, Provider: provider, ModelGroup: groupName, Now: now, Policy: state.ProbePolicy{}})
		if err != nil {
			return nil, err
		}
		if needsProbe {
			result = append(result, probe)
		}
	}
	for _, group := range append(plan.ActiveGroups, plan.DisabledGroups...) {
		for _, probe := range group.Probes {
			provider := filterProvider(probe.Credential)
			groupName := probeModelGroup(provider, modelGroup)
			if !probe.NextProbeAt.After(now) {
				needsProbe, err := store.NeedsProbe(ctx, state.ProbeCheck{AuthIndex: probe.Credential.AuthIndex, Provider: provider, ModelGroup: groupName, Now: now, Policy: state.ProbePolicy{}})
				if err != nil {
					return nil, err
				}
				if needsProbe {
					result = append(result, probe)
				}
				continue
			}
			if store.HasEntry(probe.Credential.AuthIndex, groupName) {
				needsProbe, err := store.NeedsProbe(ctx, state.ProbeCheck{AuthIndex: probe.Credential.AuthIndex, Provider: provider, ModelGroup: groupName, Now: now, Policy: state.ProbePolicy{}})
				if err != nil {
					return nil, err
				}
				if needsProbe {
					result = append(result, schedule.Probe{Credential: probe.Credential, NextProbeAt: now})
				}
				continue
			}
			if err := store.MarkProbeScheduled(ctx, state.ProbeSchedule{AuthIndex: probe.Credential.AuthIndex, Provider: provider, ModelGroup: groupName, NextProbeAt: probe.NextProbeAt}); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
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
	return schedule.Options{Clock: fixedClock{now: now}, RNG: realRNG{}, ImmediateProbeLimit: cfg.ImmediateProbeLimit, TopPriorityProbeCount: cfg.TopPriorityProbeCount, ActiveGroupSize: cfg.ActiveGroupSize, ActiveGroupJitter: cfg.ActiveGroupJitter, DisabledGroupSize: cfg.DisabledGroupSize, DisabledProbeInterval: cfg.DisabledProbeInterval}
}

func priorityOptions(cfg config.Config, now time.Time) priority.Options {
	options := priority.Options{Now: now, MaxPriority: 100, MinChange: cfg.MinChange, PaidFirst: true, ResetBoostWithin: 5 * time.Hour, ResetBoost: 50}
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

type realRNG struct{}

func (realRNG) Int63n(limit int64) int64 {
	return rand.Int63n(limit)
}

func (r *Runtime) SaveSnapshot(ctx context.Context, snapshot apply.PlanSnapshot) error {
	return ctx.Err()
}

func (r *Runtime) RecordEvent(ctx context.Context, event apply.AuditEvent) error {
	return ctx.Err()
}

var _ apply.Auditor = (*Runtime)(nil)
