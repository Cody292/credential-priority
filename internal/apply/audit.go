package apply

import (
	"encoding/json"
	"strconv"

	"credential-priority/internal/core"
	"credential-priority/internal/host"
	"credential-priority/internal/priority"
)

// PlanSnapshot 是写入前可安全保存的计划快照。
type PlanSnapshot struct {
	TotalItems   int              `json:"total_items"`
	TotalChanges int              `json:"total_changes"`
	Items        []SnapshotItem   `json:"items"`
	Changes      []SnapshotChange `json:"changes"`
}

// Snapshot 返回 plan 的脱敏快照，供 dry-run 和诊断路径复用。
func Snapshot(plan priority.Plan) PlanSnapshot {
	return newPlanSnapshot(plan)
}

// SnapshotItem 是单个计划项的脱敏审计视图。
type SnapshotItem struct {
	Name          string `json:"name"`
	AuthIndex     string `json:"auth_index"`
	Provider      string `json:"provider"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Current       Target `json:"current"`
	Target        Target `json:"target"`
	EvidenceFresh bool   `json:"evidence_fresh"`
	Reason        string `json:"reason"`
}

// SnapshotChange 是单个写入候选的脱敏审计视图。
type SnapshotChange struct {
	Name          string `json:"name"`
	AuthIndex     string `json:"auth_index"`
	Current       Target `json:"current"`
	Target        Target `json:"target"`
	EvidenceFresh bool   `json:"evidence_fresh"`
	Reason        string `json:"reason"`
}

// Target 表示凭证的优先级和禁用目标状态。
type Target struct {
	Priority int  `json:"priority"`
	Disabled bool `json:"disabled"`
}

// AuditEvent 是写入开始前记录的脱敏审计事件。
type AuditEvent struct {
	Action         string        `json:"action"`
	TotalChanges   int           `json:"total_changes"`
	FreshChanges   int           `json:"fresh_changes"`
	SkippedChanges int           `json:"skipped_changes"`
	Changes        []AuditChange `json:"changes"`
}

// AuditChange 是审计事件中的单个写入候选摘要。
type AuditChange struct {
	Name          string `json:"name"`
	AuthIndex     string `json:"auth_index"`
	EvidenceFresh bool   `json:"evidence_fresh"`
	Reason        string `json:"reason"`
}

func newPlanSnapshot(plan priority.Plan) PlanSnapshot {
	snapshot := PlanSnapshot{
		TotalItems:   len(plan.Items),
		TotalChanges: len(plan.Changes),
		Items:        make([]SnapshotItem, 0, len(plan.Items)),
		Changes:      make([]SnapshotChange, 0, len(plan.Changes)),
	}
	for _, item := range plan.Items {
		snapshot.Items = append(snapshot.Items, snapshotItem(item))
	}
	for _, change := range plan.Changes {
		snapshot.Changes = append(snapshot.Changes, snapshotChange(change))
	}
	return snapshot
}

func newAuditEvent(plan priority.Plan) AuditEvent {
	event := AuditEvent{
		Action:       "apply.plan",
		TotalChanges: len(plan.Changes),
		Changes:      make([]AuditChange, 0, len(plan.Changes)),
	}
	for _, change := range plan.Changes {
		if change.EvidenceFresh {
			event.FreshChanges++
		} else {
			event.SkippedChanges++
		}
		event.Changes = append(event.Changes, AuditChange{
			Name:          redactString(change.Credential.Name),
			AuthIndex:     redactString(change.Credential.AuthIndex),
			EvidenceFresh: change.EvidenceFresh,
			Reason:        redactString(change.Reason),
		})
	}
	return event
}

func snapshotItem(item priority.PlanItem) SnapshotItem {
	credential := item.Credential
	return SnapshotItem{
		Name:          redactString(credential.Name),
		AuthIndex:     redactString(credential.AuthIndex),
		Provider:      redactString(string(credential.Provider)),
		Type:          redactString(string(credential.Type)),
		Status:        redactString(string(credential.Status)),
		Current:       target(credential.Priority, credential.Disabled),
		Target:        target(item.Priority, item.Disabled),
		EvidenceFresh: item.EvidenceFresh,
		Reason:        redactString(item.Reason),
	}
}

func snapshotChange(change priority.Change) SnapshotChange {
	credential := change.Credential
	return SnapshotChange{
		Name:          redactString(credential.Name),
		AuthIndex:     redactString(credential.AuthIndex),
		Current:       target(credential.Priority, credential.Disabled),
		Target:        target(change.Priority, change.Disabled),
		EvidenceFresh: change.EvidenceFresh,
		Reason:        redactString(change.Reason),
	}
}

func resultName(credential core.Credential) string {
	if credential.Name != "" {
		return redactString(credential.Name)
	}
	return redactString(credential.AuthIndex)
}

func target(priority int, disabled bool) Target {
	return Target{Priority: priority, Disabled: disabled}
}

func redactString(value string) string {
	if value == "" {
		return ""
	}
	return host.RedactBytes([]byte(value))
}

func redactedError(err error) string {
	if err == nil {
		return ""
	}
	return redactString(err.Error())
}

func redactedErrors(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	encoded, err := json.Marshal(errStrings(errs))
	if err != nil {
		return host.RedactedValue
	}
	return redactString(string(encoded))
}

func errStrings(errs []error) []string {
	values := make([]string, 0, len(errs))
	for index, err := range errs {
		values = append(values, strconv.Itoa(index+1)+": "+redactedError(err))
	}
	return values
}
