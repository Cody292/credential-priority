package runtime

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
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
	// defaultProbeCacheTTL 是非 xAI 探测证据 freshness（包内常量，不可配置）。
	defaultProbeCacheTTL = 15 * time.Minute
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
	store, err := state.Load(ctx, config.DefaultStateCachePath)
	if err != nil {
		return err
	}
	probes, err := probesForRequest(ctx, store, credentials, scheduleOptions(request.Config, now), request.AuthIndexes, request.Config.AntigravityModelGroup, request.Trigger)
	if err != nil {
		return err
	}
	evidence, err := r.collectEvidenceForTrigger(ctx, collectInput{client: client, store: store, probes: probes, accountIDs: accountIDs, authMaterials: authMaterials, now: now, cacheTTL: defaultProbeCacheTTL, forceProbe: request.Trigger == TriggerManualApply, maxConcurrency: request.Config.MaxConcurrency, antigravityModelGroup: request.Config.AntigravityModelGroup}, request.Trigger)
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
		providerEntries := runHistoryProvidersFromResult(result)
		r.snapshotRunEntry(result, "dry-run plan generated", RunHistoryEntry{
			Kind:      "dry_run",
			Trigger:   string(request.Trigger),
			Attempted: result.Attempted,
			Succeeded: result.Succeeded,
			Failed:    result.Failed,
			Skipped:   result.Skipped,
			Providers: providerEntries,
			Message:   "dry-run plan generated",
		})
		return nil
	}
	result, err := apply.Apply(ctx, apply.Request{Host: client, Auditor: r, Plan: plan, ReportSkippedPlan: true})
	if err != nil {
		return err
	}
	providerEntries := runHistoryProvidersFromResult(result)
	summary := fmt.Sprintf("apply credentials=%d succeeded=%d failed=%d skipped=%d", result.Attempted+result.Skipped, result.Succeeded, result.Failed, result.Skipped)
	r.snapshotRunEntry(result, summary, RunHistoryEntry{
		Kind:      "apply",
		Trigger:   string(request.Trigger),
		Attempted: result.Attempted,
		Succeeded: result.Succeeded,
		Failed:    result.Failed,
		Skipped:   result.Skipped,
		Providers: providerEntries,
		Message:   summary,
	})
	return nil
}

// runAutoParallelProviders 在一轮自动任务内按 provider 并行探测与写回。
// 共享同一 state.Store，结束后统一 SaveAtomic，避免并行 Load/Save 互相覆盖。
// 调用方必须已持有 runMu（AutoApply 入口）。
func (r *Runtime) runAutoParallelProviders(ctx context.Context, request TaskRequest) error {
	if r.hostCallbacks == nil {
		return errMissingHostCallbacks
	}
	now := r.clock.Now().UTC()
	client := host.NewClient(r.hostCallbacks)
	files, err := client.ListAuthFiles(ctx)
	if err != nil {
		return err
	}
	allCredentials, accountIDs := credentialsFromAuthFiles(files)
	allCredentials = filterCredentialsByProvider(allCredentials, request.Config)
	allCredentials = filterCredentialsByAuthIndex(allCredentials, request.AuthIndexes)
	allCredentials, authMaterials, err := enrichCredentialsFromAuthDocuments(ctx, client, allCredentials)
	if err != nil {
		return err
	}
	store, err := state.Load(ctx, config.DefaultStateCachePath)
	if err != nil {
		return err
	}

	providers := autoProvidersFromCredentials(allCredentials, request.Config)
	if len(providers) == 0 {
		r.snapshotRunEntry(apply.Result{}, "auto_apply no supported providers", RunHistoryEntry{
			Kind:    "auto_apply",
			Trigger: string(TriggerAutoApply),
			Message: "auto_apply no supported providers",
		})
		return nil
	}

	type providerResult struct {
		provider core.Provider
		result   apply.Result
		err      error
	}
	results := make(chan providerResult, len(providers))
	var wg sync.WaitGroup
	for _, providerName := range providers {
		providerName := providerName
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider := core.Provider(providerName)
			credentials := filterCredentialsToProvider(allCredentials, provider)
			if len(credentials) == 0 {
				results <- providerResult{provider: provider}
				return
			}
			probes, err := probesForRequest(ctx, store, credentials, scheduleOptions(request.Config, now), request.AuthIndexes, request.Config.AntigravityModelGroup, TriggerAutoApply)
			if err != nil {
				results <- providerResult{provider: provider, err: err}
				return
			}
			evidence, err := r.collectEvidenceForTrigger(ctx, collectInput{
				client: client, store: store, probes: probes, accountIDs: accountIDs, authMaterials: authMaterials,
				now: now, cacheTTL: defaultProbeCacheTTL, forceProbe: false,
				maxConcurrency: request.Config.MaxConcurrency, antigravityModelGroup: request.Config.AntigravityModelGroup,
			}, TriggerAutoApply)
			if err != nil {
				results <- providerResult{provider: provider, err: err}
				return
			}
			plan := priority.PlanFreshOnly(credentials, evidence, priorityOptions(request.Config, now))
			plan = withProbeFailureTemporaryDisables(plan, evidence)
			result, err := apply.Apply(ctx, apply.Request{Host: client, Auditor: r, Plan: plan, ReportSkippedPlan: true})
			results <- providerResult{provider: provider, result: result, err: err}
		}()
	}
	wg.Wait()
	close(results)

	var (
		firstErr  error
		attempted int
		succeeded int
		failed    int
		skipped   int
	)
	parts := make([]string, 0, len(providers))
	providerEntries := make([]RunHistoryProvider, 0, len(providers))
	for item := range results {
		errText := ""
		if item.err != nil {
			errText = item.err.Error()
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", item.provider, item.err)
			}
		}
		attempted += item.result.Attempted
		succeeded += item.result.Succeeded
		failed += item.result.Failed
		skipped += item.result.Skipped
		parts = append(parts, fmt.Sprintf("%s attempted=%d succeeded=%d failed=%d skipped=%d", item.provider, item.result.Attempted, item.result.Succeeded, item.result.Failed, item.result.Skipped))
		providerEntries = append(providerEntries, RunHistoryProvider{
			Name:      string(item.provider),
			Attempted: item.result.Attempted,
			Succeeded: item.result.Succeeded,
			Failed:    item.result.Failed,
			Skipped:   item.result.Skipped,
			Error:     errText,
		})
	}
	summary := "auto_apply parallel: " + strings.Join(parts, "; ")
	result := apply.Result{Attempted: attempted, Succeeded: succeeded, Failed: failed, Skipped: skipped}
	// 先写 history，再 SaveAtomic：避免 SaveAtomic/ctx 失败导致「无记录却算跑过」。
	r.snapshotRunEntry(result, summary, RunHistoryEntry{
		Kind:      "auto_apply",
		Trigger:   string(TriggerAutoApply),
		Attempted: attempted,
		Succeeded: succeeded,
		Failed:    failed,
		Skipped:   skipped,
		Providers: providerEntries,
		Message:   summary,
	})
	if err := store.SaveAtomic(ctx); err != nil {
		return err
	}
	return firstErr
}

// runHistoryProvidersFromResult 将 apply 结果按提供商分桶，供执行记录 UI 与自动排序对齐展示。
func runHistoryProvidersFromResult(result apply.Result) []RunHistoryProvider {
	order := []string{string(core.ProviderAntigravity), string(core.ProviderCodex), string(core.ProviderXAI)}
	buckets := make(map[string]*RunHistoryProvider, len(order))
	for _, name := range order {
		buckets[name] = &RunHistoryProvider{Name: name}
	}
	for _, change := range result.Changes {
		name := strings.ToLower(strings.TrimSpace(change.Provider))
		if name == "" {
			continue
		}
		bucket, ok := buckets[name]
		if !ok {
			bucket = &RunHistoryProvider{Name: name}
			buckets[name] = bucket
			order = append(order, name)
		}
		switch change.Status {
		case apply.ChangeStatusSuccess:
			bucket.Attempted++
			bucket.Succeeded++
		case apply.ChangeStatusFailed:
			bucket.Attempted++
			bucket.Failed++
		case apply.ChangeStatusSkipped:
			bucket.Skipped++
		}
	}
	out := make([]RunHistoryProvider, 0, len(order))
	for _, name := range order {
		bucket := buckets[name]
		if bucket == nil {
			continue
		}
		if bucket.Attempted == 0 && bucket.Succeeded == 0 && bucket.Failed == 0 && bucket.Skipped == 0 {
			continue
		}
		out = append(out, *bucket)
	}
	return out
}

func autoProvidersFromCredentials(credentials []core.Credential, cfg config.Config) []string {
	order := []string{string(core.ProviderAntigravity), string(core.ProviderCodex), string(core.ProviderXAI)}
	present := map[string]struct{}{}
	for _, credential := range credentials {
		p := filterProvider(credential)
		if p == core.ProviderAntigravity || p == core.ProviderCodex || p == core.ProviderXAI {
			present[string(p)] = struct{}{}
		}
	}
	selectedFilter := map[string]struct{}{}
	if cfg.ProviderScope == config.ProviderScopeSelected && len(cfg.SelectedProviders) > 0 {
		for _, provider := range cfg.SelectedProviders {
			selectedFilter[provider] = struct{}{}
		}
	}
	result := make([]string, 0, len(present))
	for _, provider := range order {
		if _, ok := present[provider]; !ok {
			continue
		}
		if len(selectedFilter) > 0 {
			if _, ok := selectedFilter[provider]; !ok {
				continue
			}
		}
		result = append(result, provider)
	}
	return result
}

func filterCredentialsToProvider(credentials []core.Credential, provider core.Provider) []core.Credential {
	filtered := make([]core.Credential, 0, len(credentials))
	for _, credential := range credentials {
		if filterProvider(credential) == provider {
			filtered = append(filtered, credential)
		}
	}
	return filtered
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
			// xAI：无可信额度信号时必须保持现状，禁止因 probe_failed 临时禁用。
			if item.Provider == core.ProviderXAI {
				continue
			}
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
		if filterProvider(item.Credential) == core.ProviderXAI {
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
			if p == core.ProviderAntigravity || p == core.ProviderCodex || p == core.ProviderXAI {
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
	case core.CredentialTypeXAI:
		return core.ProviderXAI
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
	// disabled 分批改用 Interval + ActiveGroupSize；不再传入有效 DisabledProbeInterval。
	return schedule.Options{
		Clock:                 fixedClock{now: now},
		RNG:                   realRNG{},
		ImmediateProbeLimit:   cfg.ImmediateProbeLimit,
		TopPriorityProbeCount: cfg.TopPriorityProbeCount,
		ActiveGroupSize:       cfg.ActiveGroupSize,
		ActiveGroupJitter:     cfg.ActiveGroupJitter,
		Interval:              cfg.Interval,
	}
}

func priorityOptions(cfg config.Config, now time.Time) priority.Options {
	options := priority.Options{Now: now, MaxPriority: 100, MinChange: cfg.MinChange, PaidFirst: true, ResetBoostWithin: 24 * time.Hour, ResetBoost: 50}
	if cfg.PriorityRules.Enabled {
		freeDepletedPriority := cfg.PriorityRules.Codex.FreeDepletedPriority
		freeDepletedDisabled := cfg.PriorityRules.Codex.FreeDepletedDisabled
		paidDepletedDisabled := cfg.PriorityRules.Codex.PaidDepletedDisabled
		xaiFreePriority := cfg.PriorityRules.XAI.FreeDepletedPriority
		xaiFreeDisabled := cfg.PriorityRules.XAI.FreeDepletedDisabled
		xaiFreeParticipates := cfg.PriorityRules.XAI.FreeParticipatesPriority
		xaiWeeklyPriority := cfg.PriorityRules.XAI.WeeklyDepletedPriority
		xaiMonthlyWeeklyPriority := cfg.PriorityRules.XAI.MonthlyAndWeeklyDepletedPriority
		xaiMonthlyWeeklyDisabled := cfg.PriorityRules.XAI.MonthlyAndWeeklyDepletedDisabled
		options.StartPriorityByProvider = map[core.Provider]int{
			core.ProviderAntigravity: cfg.PriorityRules.Antigravity.StartPriority,
			core.ProviderCodex:       cfg.PriorityRules.Codex.StartPriority,
			core.ProviderXAI:         cfg.PriorityRules.XAI.StartPriority,
		}
		options.CodexFreeDepletedPriority = &freeDepletedPriority
		options.CodexFreeDepletedDisabled = &freeDepletedDisabled
		options.CodexPaidDepletedDisabled = &paidDepletedDisabled
		options.XAIFreeDepletedPriority = &xaiFreePriority
		options.XAIFreeDepletedDisabled = &xaiFreeDisabled
		options.XAIFreeParticipatesPriority = &xaiFreeParticipates
		options.XAIWeeklyDepletedPriority = &xaiWeeklyPriority
		options.XAIMonthlyAndWeeklyDepletedPriority = &xaiMonthlyWeeklyPriority
		options.XAIMonthlyAndWeeklyDepletedDisabled = &xaiMonthlyWeeklyDisabled
	}
	return options
}

func probePolicy(cacheTTL time.Duration) state.ProbePolicy {
	return state.ProbePolicy{TTL: cacheTTL, ResetStaleAfter: time.Hour}
}

// probePolicyForProvider：xAI 使用 24h TTL，避免默认 15m 覆盖 NextProbeAt 导致狂探。
// 其它 provider 使用包内常量 defaultProbeCacheTTL（15m）。
func probePolicyForProvider(provider core.Provider, cacheTTL time.Duration) state.ProbePolicy {
	if provider == core.ProviderXAI {
		return state.ProbePolicy{TTL: xaiPositiveProbeInterval, ResetStaleAfter: time.Hour}
	}
	return probePolicy(cacheTTL)
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
