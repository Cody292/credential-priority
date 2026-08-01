package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"credential-priority/internal/config"
	"credential-priority/internal/core"
	"credential-priority/internal/host"
	"credential-priority/internal/priority"
	"credential-priority/internal/provider"
	"credential-priority/internal/provider/antigravity"
	"credential-priority/internal/provider/codex"
	"credential-priority/internal/provider/xai"
	"credential-priority/internal/schedule"
	"credential-priority/internal/state"
)

type collectInput struct {
	client                *host.Client
	store                 *state.Store
	probes                []schedule.Probe
	accountIDs            map[string]string
	authMaterials         map[string]authMaterial
	now                   time.Time
	cacheTTL              time.Duration
	forceProbe            bool
	maxConcurrency        int
	antigravityModelGroup config.AntigravityModelGroup
}

type probeJob struct {
	strategy     core.StrategyName
	provider     core.Provider
	credential   core.Credential
	accountID    string
	authMaterial authMaterial
}

func collectFreshEvidence(ctx context.Context, input collectInput) ([]priority.ProbeEvidence, error) {
	registry := provider.NewRegistry()
	probers := probeSet{codex: codex.NewProber(input.client, fixedClock{now: input.now}), antigravity: antigravity.NewProber(input.client, fixedClock{now: input.now}), xai: xai.NewProber(input.client, fixedClock{now: input.now})}
	jobs := make([]probeJob, 0, len(input.probes))
	for _, probe := range input.probes {
		registryResult := registry.Evaluate(probe.Credential)
		if !probeStrategySupported(registryResult.StrategyName) {
			continue
		}
		needsProbe, err := freshProbeNeeded(ctx, input, probe.Credential.AuthIndex, registryResult.Provider, probeModelGroup(registryResult.Provider, input.antigravityModelGroup))
		if err != nil {
			return nil, err
		}
		if !needsProbe {
			continue
		}
		jobs = append(jobs, probeJob{
			strategy:     registryResult.StrategyName,
			provider:     registryResult.Provider,
			credential:   probe.Credential,
			accountID:    input.accountIDs[probe.Credential.AuthIndex],
			authMaterial: input.authMaterials[probe.Credential.AuthIndex],
		})
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return runProbeJobs(ctx, probers, input, jobs)
}

func runProbeJobs(ctx context.Context, probers probeSet, input collectInput, jobs []probeJob) ([]priority.ProbeEvidence, error) {
	workers := input.maxConcurrency
	if workers < 1 {
		workers = 2
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers == 1 {
		evidence := make([]priority.ProbeEvidence, 0, len(jobs))
		for _, job := range jobs {
			item, err := probers.probeAndRecord(ctx, probeAndRecordInput{
				store: input.store, client: input.client, strategy: job.strategy, provider: job.provider, credential: job.credential,
				accountID: job.accountID, authMaterial: job.authMaterial, now: input.now, antigravityModelGroup: input.antigravityModelGroup,
			})
			if err != nil {
				return nil, err
			}
			if item.Status != priority.EvidenceStatusUnknown {
				evidence = append(evidence, item)
			}
		}
		return evidence, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		index int
		item  priority.ProbeEvidence
		err   error
	}
	jobsCh := make(chan int)
	resultsCh := make(chan result, len(jobs))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobsCh {
				if runCtx.Err() != nil {
					return
				}
				job := jobs[index]
				item, err := probers.probeAndRecord(runCtx, probeAndRecordInput{
					store: input.store, client: input.client, strategy: job.strategy, provider: job.provider, credential: job.credential,
					accountID: job.accountID, authMaterial: job.authMaterial, now: input.now, antigravityModelGroup: input.antigravityModelGroup,
				})
				select {
				case resultsCh <- result{index: index, item: item, err: err}:
				case <-runCtx.Done():
					return
				}
				if err != nil {
					cancel()
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobsCh)
		for index := range jobs {
			select {
			case jobsCh <- index:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	ordered := make([]priority.ProbeEvidence, len(jobs))
	present := make([]bool, len(jobs))
	var firstErr error
	for res := range resultsCh {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			cancel()
			continue
		}
		if res.item.Status != priority.EvidenceStatusUnknown {
			ordered[res.index] = res.item
			present[res.index] = true
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	evidence := make([]priority.ProbeEvidence, 0, len(jobs))
	for index, ok := range present {
		if ok {
			evidence = append(evidence, ordered[index])
		}
	}
	return evidence, nil
}

func probeStrategySupported(strategy core.StrategyName) bool {
	return strategy == core.StrategyCodex || strategy == core.StrategyChatGPT || strategy == core.StrategyAntigravity || strategy == core.StrategyXAI
}

func freshProbeNeeded(ctx context.Context, input collectInput, authIndex string, provider core.Provider, modelGroup string) (bool, error) {
	if input.forceProbe {
		return true, nil
	}
	return input.store.NeedsProbe(ctx, state.ProbeCheck{
		AuthIndex:  authIndex,
		Provider:   provider,
		ModelGroup: modelGroup,
		Now:        input.now,
		Policy:     probePolicyForProvider(provider, input.cacheTTL),
	})
}

func probeModelGroup(provider core.Provider, modelGroup config.AntigravityModelGroup) string {
	if provider != core.ProviderAntigravity {
		return ""
	}
	return string(modelGroup)
}

type probeSet struct {
	codex       codex.Prober
	antigravity antigravity.Prober
	xai         xai.Prober
}

type probeAndRecordInput struct {
	store                 *state.Store
	client                *host.Client
	strategy              core.StrategyName
	provider              core.Provider
	credential            core.Credential
	accountID             string
	authMaterial          authMaterial
	now                   time.Time
	antigravityModelGroup config.AntigravityModelGroup
}

func (p probeSet) probeAndRecord(ctx context.Context, input probeAndRecordInput) (priority.ProbeEvidence, error) {
	if input.strategy == core.StrategyAntigravity {
		result := p.antigravity.Probe(ctx, antigravity.ProbeRequest{AuthIndex: input.credential.AuthIndex, AccessToken: input.authMaterial.accessToken, ProjectID: input.authMaterial.projectID, ModelGroup: input.antigravityModelGroup})
		return recordAntigravityProbeResult(ctx, input.store, result, input.now)
	}
	if input.strategy == core.StrategyXAI {
		return p.probeAndRecordXAI(ctx, input)
	}
	accountID := input.accountID
	if accountID == "" {
		accountID = input.authMaterial.accountID
	}
	result := p.codex.Probe(ctx, codex.ProbeRequest{Provider: input.provider, AuthIndex: input.credential.AuthIndex, AccountID: accountID, AccessToken: input.authMaterial.accessToken})
	return recordCodexProbeResult(ctx, input.store, result, input.now)
}

func (p probeSet) probeAndRecordXAI(ctx context.Context, input probeAndRecordInput) (priority.ProbeEvidence, error) {
	accessToken := input.authMaterial.accessToken
	localExpired := xaiAuthMaterialLooksExpired(ctx, input.client, input.credential.AuthIndex, accessToken, input.now)
	// 探测前：常规 refresh；本地 JWT/expired 已过期则强制 refresh 一次。
	if token, ok := maybeRefreshXAIAuth(ctx, input.client, input.credential.AuthIndex, input.credential.Disabled, localExpired, input.now); ok {
		accessToken = token
		localExpired = xai.AccessTokenExpired(accessToken, input.now)
	}
	// Plan classification only (settings/billing/JWT). No multi-model chat probe.
	plan := p.xai.FetchPlan(ctx, xai.PlanRequest{
		AuthIndex:   input.credential.AuthIndex,
		AccessToken: accessToken,
		BaseURL:     input.authMaterial.baseURL,
	})
	// 401/凭证文案 或 本地已过期仍拿不到有效 plan：force refresh 一次后重拉 plan。
	needForceRefresh := planLooksUnauthorized(plan) || (localExpired && plan.Source == "default_unfetchable")
	if needForceRefresh {
		if token, ok := maybeRefreshXAIAuth(ctx, input.client, input.credential.AuthIndex, input.credential.Disabled, true, input.now); ok {
			accessToken = token
			plan = p.xai.FetchPlan(ctx, xai.PlanRequest{
				AuthIndex:   input.credential.AuthIndex,
				AccessToken: accessToken,
				BaseURL:     input.authMaterial.baseURL,
			})
			localExpired = xai.AccessTokenExpired(accessToken, input.now)
		}
		// force refresh 后仍鉴权失败，或本地仍过期且仍 unfetchable → 硬 AuthInvalid
		if planLooksUnauthorized(plan) {
			return recordXAIPlanResult(ctx, input.store, plan, input.now)
		}
		if localExpired && (plan.AuthFailed || plan.Source == "default_unfetchable" || plan.Source == "auth_failed") {
			plan.AuthFailed = true
			plan.PlanClass = xai.PlanClassUnknown
			plan.PlanType = core.PlanTypeUnknown
			plan.Source = "auth_failed"
			if plan.Error == "" {
				plan.Error = "local token expired after refresh"
			}
			if plan.HTTPStatus == 0 {
				plan.HTTPStatus = 401
			}
		}
	}
	return recordXAIPlanResult(ctx, input.store, plan, input.now)
}

func planLooksUnauthorized(plan xai.PlanResult) bool {
	if plan.AuthFailed {
		return true
	}
	if xai.IsUnauthorizedProbe(plan.HTTPStatus, plan.Error) {
		return true
	}
	return false
}

// xaiAuthMaterialLooksExpired 读取 auth JSON expired + JWT exp，判断本地凭证是否已过期。
func xaiAuthMaterialLooksExpired(ctx context.Context, client *host.Client, authIndex, accessToken string, now time.Time) bool {
	if xai.AccessTokenExpired(accessToken, now) {
		return true
	}
	if client == nil || strings.TrimSpace(authIndex) == "" {
		return false
	}
	raw, err := readCredentialAuthJSON(ctx, client, authIndex)
	if err != nil || len(raw) == 0 {
		return false
	}
	fields, err := xai.ParseAuthRefreshFields(raw)
	if err != nil {
		return false
	}
	token := accessToken
	if token == "" {
		token = fields.AccessToken
	}
	return xai.AuthMaterialExpired(token, fields.ExpiredAt, now)
}

func recordCodexProbeResult(ctx context.Context, store *state.Store, result codex.ProbeResult, now time.Time) (priority.ProbeEvidence, error) {
	if result.Status != codex.StatusReady || result.ResetAt == nil || result.Remaining == nil {
		err := store.MarkProbeFailure(ctx, state.ProbeFailure{AuthIndex: result.AuthIndex, Provider: result.Provider, ObservedAt: now, Err: errors.New(result.Error), NextProbeAt: now.Add(time.Hour)})
		return priority.ProbeEvidence{Provider: result.Provider, AuthIndex: result.AuthIndex, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusProbeFailed}, err
	}
	err := store.MarkProbeSuccess(ctx, state.ProbeSuccess{AuthIndex: result.AuthIndex, Provider: result.Provider, ObservedAt: result.ObservedAt, ResetAt: *result.ResetAt, Remaining: int(*result.Remaining), Source: state.SourceFreshProbe, NextProbeAt: result.ObservedAt.Add(time.Hour)})
	return priority.ProbeEvidence{Provider: result.Provider, AuthIndex: result.AuthIndex, ObservedAt: result.ObservedAt, ResetAt: result.ResetAt, Remaining: result.Remaining, LongWindowResetAt: result.LongWindowResetAt, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusReady, PlanType: result.PlanType, EvidenceFresh: true}, err
}

func recordAntigravityProbeResult(ctx context.Context, store *state.Store, result antigravity.ProbeResult, now time.Time) (priority.ProbeEvidence, error) {
	if result.Status != antigravity.StatusReady || result.ResetAt == nil || result.Remaining == nil {
		err := store.MarkProbeFailure(ctx, state.ProbeFailure{AuthIndex: result.AuthIndex, Provider: core.ProviderAntigravity, ModelGroup: string(result.ModelGroup), ObservedAt: now, Err: errors.New(result.Error), NextProbeAt: now.Add(time.Hour)})
		return priority.ProbeEvidence{Provider: core.ProviderAntigravity, AuthIndex: result.AuthIndex, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusProbeFailed}, err
	}
	err := store.MarkProbeSuccess(ctx, state.ProbeSuccess{AuthIndex: result.AuthIndex, Provider: core.ProviderAntigravity, ModelGroup: string(result.ModelGroup), ObservedAt: result.ObservedAt, ResetAt: *result.ResetAt, Remaining: int(*result.Remaining), Source: state.SourceFreshProbe, NextProbeAt: result.ObservedAt.Add(time.Hour)})
	return priority.ProbeEvidence{Provider: core.ProviderAntigravity, AuthIndex: result.AuthIndex, ObservedAt: result.ObservedAt, ResetAt: result.ResetAt, Remaining: result.Remaining, LongWindowResetAt: result.LongWindowResetAt, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusReady, PlanType: result.PlanType, EvidenceFresh: true}, err
}

