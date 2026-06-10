package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ArgoCDAdapter 解析 ArgoCD webhook payload。
// 支持的事件类型：
//   - Sync succeeded / failed
//   - Health changed
//   - Rollout notification
type ArgoCDAdapter struct {
	secret string
}

func NewArgoCDAdapter(secret ...string) *ArgoCDAdapter {
	a := &ArgoCDAdapter{}
	if len(secret) > 0 {
		a.secret = strings.TrimSpace(secret[0])
	}
	return a
}

func (a *ArgoCDAdapter) Name() string {
	return "argocd"
}

func (a *ArgoCDAdapter) Parse(ctx context.Context, headers http.Header, body []byte) (*protocol.ChangeEvent, error) {
	if err := verifyArgoCDToken(a.secret, headers); err != nil {
		return nil, err
	}
	// ArgoCD 发送的是统一的 JSON payload，通过 content 判断事件类型
	var payload argoCDPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse argocd: %w", err)
	}

	return a.convert(payload)
}

// argoCDPayload 是 ArgoCD webhook 的通用结构。
type argoCDPayload struct {
	Spec struct {
		Source struct {
			RepoURL        string `json:"repoURL"`
			Path           string `json:"path"`
			TargetRevision string `json:"targetRevision"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
	} `json:"spec"`
	Status struct {
		Phase  string `json:"phase"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
		Sync struct {
			Status     string `json:"status"`
			Revision   string `json:"revision"`
			ComparedTo struct {
				Source struct {
					TargetRevision string `json:"targetRevision"`
				} `json:"source"`
			} `json:"comparedTo"`
		} `json:"sync"`
		OperationState struct {
			Phase      string `json:"phase"`
			Message    string `json:"message"`
			FinishedAt string `json:"finishedAt"`
			RetryCount int64  `json:"retryCount"`
		} `json:"operationState"`
		History []struct {
			Revision   string `json:"revision"`
			DeployedAt string `json:"deployedAt"`
		} `json:"history"`
	} `json:"status"`
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
}

func (a *ArgoCDAdapter) convert(payload argoCDPayload) (*protocol.ChangeEvent, error) {
	appName := payload.Metadata.Name
	if appName == "" {
		return nil, fmt.Errorf("argocd: missing application name")
	}

	// 提取服务名：从 application name 或 source path
	service := appName
	if payload.Spec.Source.Path != "" {
		// 路径格式通常是 apps/service-name 或 k8s/service-name
		parts := strings.Split(strings.Trim(payload.Spec.Source.Path, "/"), "/")
		if len(parts) > 1 {
			service = parts[len(parts)-1]
		}
	}

	env := payload.Spec.Destination.Namespace
	if env == "" {
		env = "unknown"
	} else {
		env = normalizeEnv(env)
	}

	cluster := payload.Spec.Destination.Server

	// 判断事件类型和状态
	phase := payload.Status.OperationState.Phase
	syncStatus := payload.Status.Sync.Status
	healthStatus := payload.Status.Health.Status

	// 确定变更类型
	eventType := protocol.ChangeTypeDeploy
	summary := ""

	switch {
	case phase == "Succeeded":
		eventType = protocol.ChangeTypeDeploy
		revision := payload.Status.Sync.Revision
		if revision == "" && len(payload.Status.History) > 0 {
			revision = payload.Status.History[len(payload.Status.History)-1].Revision
		}
		summary = fmt.Sprintf("ArgoCD sync succeeded: %s → %s", appName, shortRevision(revision))

	case phase == "Failed":
		eventType = protocol.ChangeTypeDeploy
		summary = fmt.Sprintf("ArgoCD sync failed: %s - %s", appName, payload.Status.OperationState.Message)

	case phase == "Error":
		eventType = protocol.ChangeTypeDeploy
		summary = fmt.Sprintf("ArgoCD sync error: %s - %s", appName, payload.Status.OperationState.Message)

	case syncStatus == "OutOfSync":
		eventType = protocol.ChangeTypeConfigUpdate
		summary = fmt.Sprintf("ArgoCD out of sync: %s", appName)

	case healthStatus == "Degraded":
		eventType = protocol.ChangeTypeDeploy
		summary = fmt.Sprintf("ArgoCD health degraded: %s", appName)

	default:
		summary = fmt.Sprintf("ArgoCD event: %s (phase=%s, sync=%s, health=%s)",
			appName, phase, syncStatus, healthStatus)
	}

	// 风险等级
	riskLevel := protocol.ChangeRiskMedium
	if phase == "Failed" || phase == "Error" || healthStatus == "Degraded" {
		riskLevel = protocol.ChangeRiskHigh
	}

	// 时间
	var startedAt time.Time
	if len(payload.Status.History) > 0 {
		lastDeploy := payload.Status.History[len(payload.Status.History)-1]
		startedAt, _ = time.Parse(time.RFC3339, lastDeploy.DeployedAt)
	}
	if startedAt.IsZero() {
		startedAt, _ = time.Parse(time.RFC3339, payload.Status.OperationState.FinishedAt)
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	// Before/After
	var before, after map[string]any
	if payload.Status.Sync.ComparedTo.Source.TargetRevision != "" {
		before = map[string]any{
			"revision": shortRevision(payload.Status.Sync.ComparedTo.Source.TargetRevision),
		}
	}
	revision := payload.Status.Sync.Revision
	if revision == "" && len(payload.Status.History) > 0 {
		revision = payload.Status.History[len(payload.Status.History)-1].Revision
	}
	if revision != "" {
		after = map[string]any{
			"revision": shortRevision(revision),
		}
	}

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceArgoCD,
		EventType:  eventType,
		Service:    service,
		Env:        env,
		Namespace:  payload.Spec.Destination.Namespace,
		Cluster:    cluster,
		Summary:    summary,
		RiskLevel:  riskLevel,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		DedupeKey:  argoCDDedupeKey(appName, phase, syncStatus, revision, startedAt),
		Before:     before,
		After:      after,
		CorrelationKeys: []string{
			strings.ToLower(service),
			env,
			cluster,
		},
		RawPayload: map[string]any{
			"platform":      "argocd",
			"app_name":      appName,
			"phase":         phase,
			"sync_status":   syncStatus,
			"health_status": healthStatus,
		},
		Metadata: map[string]any{
			"argocd_app":  appName,
			"revision":    revision,
			"destination": payload.Spec.Destination.Server,
			"namespace":   payload.Spec.Destination.Namespace,
		},
	}, nil
}

func verifyArgoCDToken(secret string, headers http.Header) error {
	token := headers.Get("X-ArgoCD-Webhook-Secret")
	if token == "" {
		token = headers.Get("X-OpsCaption-Webhook-Token")
	}
	return verifySharedToken(secret, token)
}

func argoCDDedupeKey(appName, phase, syncStatus, revision string, startedAt time.Time) string {
	parts := []string{"argocd", strings.ToLower(appName), phase, syncStatus, revision}
	if !startedAt.IsZero() {
		parts = append(parts, startedAt.UTC().Format(time.RFC3339))
	}
	return strings.Join(parts, ":")
}

func shortRevision(rev string) string {
	if len(rev) > 8 {
		return rev[:8]
	}
	return rev
}
