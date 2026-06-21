package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"credential-priority/internal/apply"
	"credential-priority/internal/config"
	"credential-priority/internal/core"
	"credential-priority/internal/host"
	"credential-priority/internal/priority"
	"credential-priority/internal/provider"
	"credential-priority/internal/provider/codex"
	"credential-priority/internal/schedule"
	"credential-priority/internal/state"
)

var errMissingHostCallbacks = errors.New("runtime: host callbacks are required")

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
	store, err := state.Load(ctx, request.Config.CachePath)
	if err != nil {
		return err
	}
	probePlan, err := schedule.PlanProbeSchedule(credentials, scheduleOptions(request.Config, now))
	if err != nil {
		return err
	}
	evidence, err := collectFreshEvidence(ctx, collectInput{client: client, store: store, probes: probePlan.Immediate, accountIDs: accountIDs, now: now, cacheTTL: request.Config.CacheTTL})
	if err != nil {
		return err
	}
	if err := store.SaveAtomic(ctx); err != nil {
		return err
	}
	plan := priority.PlanFreshOnly(credentials, evidence, priorityOptions(request.Config, now))
	if request.Trigger != TriggerAutoApply {
		result := apply.Result{Snapshot: apply.Snapshot(plan)}
		r.snapshotRun(result, "dry-run plan generated")
		return nil
	}
	result, err := apply.Apply(ctx, apply.Request{Host: client, Auditor: r, Plan: plan})
	if err != nil {
		return err
	}
	r.snapshotRun(result, fmt.Sprintf("apply attempted=%d succeeded=%d failed=%d skipped=%d", result.Attempted, result.Succeeded, result.Failed, result.Skipped))
	return nil
}

type collectInput struct {
	client     *host.Client
	store      *state.Store
	probes     []schedule.Probe
	accountIDs map[string]string
	now        time.Time
	cacheTTL   time.Duration
}

func collectFreshEvidence(ctx context.Context, input collectInput) ([]priority.ProbeEvidence, error) {
	registry := provider.NewRegistry()
	prober := codex.NewProber(input.client, fixedClock{now: input.now})
	evidence := make([]priority.ProbeEvidence, 0, len(input.probes))
	for _, probe := range input.probes {
		registryResult := registry.Evaluate(probe.Credential)
		if registryResult.StrategyName != core.StrategyCodex && registryResult.StrategyName != core.StrategyChatGPT {
			continue
		}
		needsProbe, err := input.store.NeedsProbe(ctx, state.ProbeCheck{AuthIndex: probe.Credential.AuthIndex, Provider: registryResult.Provider, Now: input.now, Policy: probePolicy(input.cacheTTL)})
		if err != nil {
			return nil, err
		}
		if !needsProbe {
			continue
		}
		result := prober.Probe(ctx, codex.ProbeRequest{Provider: registryResult.Provider, AuthIndex: probe.Credential.AuthIndex, AccountID: input.accountIDs[probe.Credential.AuthIndex]})
		item, err := recordProbeResult(ctx, input.store, result, input.now)
		if err != nil {
			return nil, err
		}
		if item.Status != priority.EvidenceStatusUnknown {
			evidence = append(evidence, item)
		}
	}
	return evidence, nil
}

func recordProbeResult(ctx context.Context, store *state.Store, result codex.ProbeResult, now time.Time) (priority.ProbeEvidence, error) {
	if result.Status != codex.StatusReady || result.ResetAt == nil || result.Remaining == nil {
		err := store.MarkProbeFailure(ctx, state.ProbeFailure{AuthIndex: result.AuthIndex, Provider: result.Provider, ObservedAt: now, Err: errors.New(result.Error), NextProbeAt: now.Add(time.Hour)})
		return priority.ProbeEvidence{Provider: result.Provider, AuthIndex: result.AuthIndex, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusProbeFailed}, err
	}
	err := store.MarkProbeSuccess(ctx, state.ProbeSuccess{AuthIndex: result.AuthIndex, Provider: result.Provider, ObservedAt: result.ObservedAt, ResetAt: *result.ResetAt, Remaining: int(*result.Remaining), Source: state.SourceFreshProbe, NextProbeAt: result.ObservedAt.Add(time.Hour)})
	return priority.ProbeEvidence{Provider: result.Provider, AuthIndex: result.AuthIndex, ObservedAt: result.ObservedAt, ResetAt: result.ResetAt, Remaining: result.Remaining, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusReady, PlanType: result.PlanType, EvidenceFresh: true}, err
}

func credentialsFromAuthFiles(files []host.AuthFile) ([]core.Credential, map[string]string) {
	credentials := make([]core.Credential, len(files))
	accountIDs := make(map[string]string, len(files))
	for index, file := range files {
		credentials[index] = core.Credential{Name: file.Name, AuthIndex: file.AuthIndex, Provider: core.Provider(file.Provider), Type: core.CredentialType(file.Type), Status: core.CredentialStatus(file.Status), Disabled: file.Disabled, Unavailable: file.Unavailable, Priority: file.Priority, Email: file.Email, PlanType: core.PlanType(file.IDToken.PlanType)}
		accountIDs[file.AuthIndex] = file.IDToken.ChatGPTAccountID
	}
	return credentials, accountIDs
}

func scheduleOptions(cfg config.Config, now time.Time) schedule.Options {
	return schedule.Options{Clock: fixedClock{now: now}, RNG: zeroRNG{}, TopPriorityProbeCount: cfg.TopPriorityProbeCount, ActiveGroupSize: cfg.ActiveGroupSize, ActiveGroupJitter: cfg.ActiveGroupJitter, DisabledGroupSize: cfg.DisabledGroupSize, DisabledProbeInterval: cfg.DisabledProbeInterval}
}

func priorityOptions(cfg config.Config, now time.Time) priority.Options {
	return priority.Options{Now: now, MaxPriority: 100, MinChange: cfg.MinChange, PaidFirst: true, ResetBoostWithin: 2 * time.Hour, ResetBoost: 50}
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
