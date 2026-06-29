package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"credential-priority/internal/core"
	"credential-priority/internal/host"
)

type authMaterial struct {
	accessToken string
	accountID   string
	projectID   string
}

func enrichCredentialsFromAuthDocuments(ctx context.Context, client *host.Client, credentials []core.Credential) ([]core.Credential, map[string]authMaterial, error) {
	enriched := append([]core.Credential(nil), credentials...)
	materials := make(map[string]authMaterial, len(credentials))
	for index, credential := range enriched {
		rawJSON, err := readCredentialAuthJSON(ctx, client, credential.AuthIndex)
		if err != nil {
			return nil, nil, err
		}
		if len(rawJSON) > 0 {
			enriched[index].RawJSON = rawJSON
			enriched[index].PriorityMissing = enriched[index].PriorityMissing || topLevelFieldMissing(rawJSON, "priority")
			enriched[index].Account = firstNonEmpty(enriched[index].Account, accountFromJSON(rawJSON), accountIDFromJSON(rawJSON))
			enriched[index].Email = firstNonEmpty(enriched[index].Email, emailFromJSON(rawJSON))
		}
		materials[credential.AuthIndex] = authMaterial{accessToken: accessTokenFromJSON(rawJSON), accountID: accountIDFromJSON(rawJSON), projectID: projectIDFromJSON(rawJSON)}
	}
	return enriched, materials, nil
}

func readCredentialAuthJSON(ctx context.Context, client *host.Client, authIndex string) (json.RawMessage, error) {
	document, err := client.GetAuth(ctx, authIndex)
	if err != nil {
		return nil, err
	}
	return physicalAuthJSON(ctx, document)
}

func physicalAuthJSON(ctx context.Context, document host.AuthDocument) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read auth document context: %w", err)
	}
	if strings.TrimSpace(document.Path) != "" {
		data, err := os.ReadFile(document.Path)
		if err != nil {
			return nil, fmt.Errorf("read auth document path: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read auth document context: %w", err)
		}
		if !json.Valid(data) {
			return nil, errors.New("auth document path contains invalid JSON")
		}
		return append(json.RawMessage(nil), data...), nil
	}
	return append(json.RawMessage(nil), document.JSON...), nil
}

func accessTokenFromJSON(raw json.RawMessage) string {
	var document struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ""
	}
	return strings.TrimSpace(document.AccessToken)
}

func projectIDFromJSON(raw json.RawMessage) string {
	var document struct {
		ProjectID      string `json:"project_id"`
		QuotaProjectID string `json:"quota_project_id"`
		Project        string `json:"project"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ""
	}
	for _, value := range []string{document.ProjectID, document.QuotaProjectID, document.Project} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func accountIDFromJSON(raw json.RawMessage) string {
	var document struct {
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ""
	}
	return strings.TrimSpace(document.AccountID)
}

func accountFromJSON(raw json.RawMessage) string {
	var document struct {
		Account string `json:"account"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ""
	}
	return strings.TrimSpace(document.Account)
}

func emailFromJSON(raw json.RawMessage) string {
	var document struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ""
	}
	return strings.TrimSpace(document.Email)
}

func topLevelFieldMissing(raw json.RawMessage, field string) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	_, ok := object[field]
	return !ok
}
