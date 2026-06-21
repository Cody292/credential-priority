package apply

import (
	"context"
	"errors"
	"fmt"

	"credential-priority/internal/priority"
)

// ChangeStatus 表示单个计划变更的执行结果。
type ChangeStatus string

const (
	// ChangeStatusSkipped 表示该变更没有 fresh 证据或没有实际差异，因此未写入宿主。
	ChangeStatusSkipped ChangeStatus = "skipped"
	// ChangeStatusSuccess 表示该 fresh 变更已完成所有必要宿主写入。
	ChangeStatusSuccess ChangeStatus = "success"
	// ChangeStatusFailed 表示该 fresh 变更至少一个宿主写入失败。
	ChangeStatusFailed ChangeStatus = "failed"
)

// ErrMissingAuditor 表示调用方未提供写入前审计持久化实现。
var ErrMissingAuditor = errors.New("apply: auditor is required")

// ErrMissingHost 表示需要写入 fresh 变更时调用方未提供宿主写入接口。
var ErrMissingHost = errors.New("apply: host is required")

// Host 是 apply writer 需要的最小宿主写入接口。
type Host interface {
	PatchPriority(ctx context.Context, name string, priority int) error
	PatchDisabled(ctx context.Context, name string, disabled bool) error
}

// Auditor 保存写入前计划快照和审计事件。
type Auditor interface {
	SaveSnapshot(ctx context.Context, snapshot PlanSnapshot) error
	RecordEvent(ctx context.Context, event AuditEvent) error
}

// Request 是执行一轮 fresh-only 写入所需的输入。
type Request struct {
	Host    Host
	Auditor Auditor
	Plan    priority.Plan
}

// Result 是一轮写入的脱敏结果汇总。
type Result struct {
	Snapshot  PlanSnapshot   `json:"snapshot"`
	Event     AuditEvent     `json:"event"`
	Changes   []ChangeResult `json:"changes"`
	Attempted int            `json:"attempted"`
	Succeeded int            `json:"succeeded"`
	Failed    int            `json:"failed"`
	Skipped   int            `json:"skipped"`
}

// ChangeResult 是单个变更的脱敏执行结果。
type ChangeResult struct {
	Name              string       `json:"name"`
	AuthIndex         string       `json:"auth_index"`
	Status            ChangeStatus `json:"status"`
	Success           bool         `json:"success"`
	EvidenceFresh     bool         `json:"evidence_fresh"`
	Reason            string       `json:"reason"`
	PriorityAttempted bool         `json:"priority_attempted"`
	DisabledAttempted bool         `json:"disabled_attempted"`
	PriorityFrom      int          `json:"priority_from"`
	PriorityTo        int          `json:"priority_to"`
	DisabledFrom      bool         `json:"disabled_from"`
	DisabledTo        bool         `json:"disabled_to"`
	Error             string       `json:"error,omitempty"`
}

// Apply 保存脱敏计划快照和审计事件，然后只写入 fresh 证据支持的变更。
func Apply(ctx context.Context, request Request) (Result, error) {
	result := Result{
		Snapshot: newPlanSnapshot(request.Plan),
		Event:    newAuditEvent(request.Plan),
		Changes:  make([]ChangeResult, 0, len(request.Plan.Changes)),
	}
	if request.Auditor == nil {
		return result, ErrMissingAuditor
	}
	if err := request.Auditor.SaveSnapshot(ctx, result.Snapshot); err != nil {
		return result, fmt.Errorf("save apply snapshot: %w", err)
	}
	if err := request.Auditor.RecordEvent(ctx, result.Event); err != nil {
		return result, fmt.Errorf("record apply audit event: %w", err)
	}
	for _, change := range request.Plan.Changes {
		changeResult := applyChange(ctx, request.Host, change)
		result.Changes = append(result.Changes, changeResult)
		summarizeChange(&result, changeResult)
	}
	return result, nil
}

func applyChange(ctx context.Context, writer Host, change priority.Change) ChangeResult {
	result := newChangeResult(change)
	if !change.EvidenceFresh {
		result.Status = ChangeStatusSkipped
		return result
	}
	priorityChanged := change.Priority != change.Credential.Priority
	disabledChanged := change.Disabled != change.Credential.Disabled
	if !priorityChanged && !disabledChanged {
		result.Status = ChangeStatusSkipped
		return result
	}
	if writer == nil {
		result.Status = ChangeStatusFailed
		result.Error = redactedError(ErrMissingHost)
		return result
	}
	errs := make([]error, 0, 2)
	if priorityChanged {
		result.PriorityAttempted = true
		if err := writer.PatchPriority(ctx, change.Credential.Name, change.Priority); err != nil {
			errs = append(errs, fmt.Errorf("patch priority: %w", err))
			result.Status = ChangeStatusFailed
			result.Error = redactedErrors(errs)
			return result
		}
	}
	if disabledChanged {
		result.DisabledAttempted = true
		if err := writer.PatchDisabled(ctx, change.Credential.Name, change.Disabled); err != nil {
			errs = append(errs, fmt.Errorf("patch disabled: %w", err))
		}
	}
	if len(errs) > 0 {
		result.Status = ChangeStatusFailed
		result.Error = redactedErrors(errs)
		return result
	}
	result.Status = ChangeStatusSuccess
	result.Success = true
	return result
}

func newChangeResult(change priority.Change) ChangeResult {
	return ChangeResult{
		Name:          resultName(change.Credential),
		AuthIndex:     redactString(change.Credential.AuthIndex),
		EvidenceFresh: change.EvidenceFresh,
		Reason:        redactString(change.Reason),
		PriorityFrom:  change.Credential.Priority,
		PriorityTo:    change.Priority,
		DisabledFrom:  change.Credential.Disabled,
		DisabledTo:    change.Disabled,
	}
}

func summarizeChange(result *Result, change ChangeResult) {
	switch change.Status {
	case ChangeStatusSuccess:
		result.Attempted++
		result.Succeeded++
	case ChangeStatusFailed:
		result.Attempted++
		result.Failed++
	case ChangeStatusSkipped:
		result.Skipped++
	}
}
