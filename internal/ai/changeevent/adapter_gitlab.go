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

// GitLabAdapter 解析 GitLab CI/CD webhook payload。
// 支持的事件类型：
//   - Pipeline Hook (pipeline 完成)
//   - Deployment Hook (部署完成)
//   - Push Hook (代码推送)
type GitLabAdapter struct {
	secret string
}

func NewGitLabAdapter(secret ...string) *GitLabAdapter {
	a := &GitLabAdapter{}
	if len(secret) > 0 {
		a.secret = strings.TrimSpace(secret[0])
	}
	return a
}

func (a *GitLabAdapter) Name() string {
	return "gitlab"
}

func (a *GitLabAdapter) Parse(ctx context.Context, headers http.Header, body []byte) (*protocol.ChangeEvent, error) {
	if err := verifySharedToken(a.secret, headers.Get("X-Gitlab-Token")); err != nil {
		return nil, err
	}
	// GitLab 使用 X-Gitlab-Event header
	eventType := headers.Get("X-Gitlab-Event")
	deliveryID := strings.TrimSpace(headers.Get("X-Gitlab-Event-UUID"))

	switch eventType {
	case "Pipeline Hook":
		return a.parsePipeline(body, deliveryID)
	case "Deployment Hook":
		return a.parseDeployment(body, deliveryID)
	case "Push Hook":
		return a.parsePush(body, deliveryID)
	default:
		return a.parseGeneric(body, eventType, deliveryID)
	}
}

// gitlabPipeline 是 GitLab Pipeline Hook 的结构。
type gitlabPipeline struct {
	ObjectKind string `json:"object_kind"`
	Attributes struct {
		ID         int64  `json:"id"`
		Ref        string `json:"ref"`
		Status     string `json:"status"`
		FinishedAt string `json:"finished_at"`
		CreatedAt  string `json:"created_at"`
		Duration   int    `json:"duration"`
	} `json:"object_attributes"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		Name              string `json:"name"`
	} `json:"project"`
	Commit struct {
		Message string `json:"message"`
		ID      string `json:"id"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
	MergeRequest struct {
		Title string `json:"title"`
	} `json:"merge_request"`
}

func (a *GitLabAdapter) parsePipeline(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload gitlabPipeline
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse gitlab pipeline: %w", err)
	}

	if payload.Attributes.Status != "success" && payload.Attributes.Status != "failed" {
		return nil, fmt.Errorf("pipeline not finished, status=%s", payload.Attributes.Status)
	}

	service := payload.Project.Name
	if parts := strings.Split(payload.Project.PathWithNamespace, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	branch := payload.Attributes.Ref
	eventType := protocol.ChangeTypePipeline
	lowerBranch := strings.ToLower(branch)
	if strings.Contains(lowerBranch, "hotfix") || strings.Contains(lowerBranch, "rollback") {
		eventType = protocol.ChangeTypeRollback
	}
	env := inferEnv(branch + " " + payload.Commit.Message)

	summary := fmt.Sprintf("GitLab Pipeline %s on %s", payload.Attributes.Status, branch)
	if payload.Commit.Message != "" {
		summary = fmt.Sprintf("GitLab Pipeline %s: %s", payload.Attributes.Status, payload.Commit.Message)
	}

	startedAt, _ := time.Parse(time.RFC3339, payload.Attributes.CreatedAt)
	finishedAt, _ := time.Parse(time.RFC3339, payload.Attributes.FinishedAt)

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitLab,
		EventType:  eventType,
		Service:    service,
		Env:        env,
		Summary:    summary,
		Operator:   payload.Commit.Author.Name,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DedupeKey:  gitlabDedupeKey(deliveryID, "pipeline", payload.Project.PathWithNamespace, fmt.Sprintf("%d:%s", payload.Attributes.ID, payload.Attributes.Status)),
		Before: map[string]any{
			"pipeline_id": payload.Attributes.ID,
		},
		After: map[string]any{
			"branch":     branch,
			"status":     payload.Attributes.Status,
			"duration_s": payload.Attributes.Duration,
		},
		RawPayload: map[string]any{
			"platform":    "gitlab",
			"event":       "pipeline",
			"project":     payload.Project.PathWithNamespace,
			"pipeline_id": payload.Attributes.ID,
		},
		Metadata: map[string]any{
			"commit_sha": payload.Commit.ID,
			"branch":     branch,
		},
	}, nil
}

// gitlabDeployment 是 GitLab Deployment Hook 的结构。
type gitlabDeployment struct {
	ObjectKind  string `json:"object_kind"`
	Status      string `json:"status"`
	Environment struct {
		Name string `json:"name"`
	} `json:"environment"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		Name              string `json:"name"`
	} `json:"project"`
	Commit struct {
		Message string `json:"message"`
		ID      string `json:"id"`
	} `json:"commit"`
	Deployment struct {
		ID        int64  `json:"id"`
		Ref       string `json:"ref"`
		CreatedAt string `json:"created_at"`
	} `json:"deployment"`
}

func (a *GitLabAdapter) parseDeployment(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload gitlabDeployment
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse gitlab deployment: %w", err)
	}

	service := payload.Project.Name
	if parts := strings.Split(payload.Project.PathWithNamespace, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	env := payload.Environment.Name
	if env == "" {
		env = "unknown"
	} else {
		env = normalizeEnv(env)
	}

	startedAt, _ := time.Parse(time.RFC3339, payload.Deployment.CreatedAt)

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitLab,
		EventType:  protocol.ChangeTypeDeploy,
		Service:    service,
		Env:        env,
		Summary:    fmt.Sprintf("GitLab Deployment %s to %s", payload.Deployment.Ref, env),
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		DedupeKey:  gitlabDedupeKey(deliveryID, "deployment", payload.Project.PathWithNamespace, fmt.Sprintf("%d:%s", payload.Deployment.ID, payload.Status)),
		After: map[string]any{
			"ref":         payload.Deployment.Ref,
			"environment": env,
			"status":      payload.Status,
		},
		RawPayload: map[string]any{
			"platform":      "gitlab",
			"event":         "deployment",
			"deployment_id": payload.Deployment.ID,
			"project":       payload.Project.PathWithNamespace,
		},
	}, nil
}

// gitlabPush 是 GitLab Push Hook 的结构。
type gitlabPush struct {
	ObjectKind string `json:"object_kind"`
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Project    struct {
		PathWithNamespace string `json:"path_with_namespace"`
		Name              string `json:"name"`
	} `json:"project"`
	Commits []struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commits"`
}

func (a *GitLabAdapter) parsePush(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload gitlabPush
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse gitlab push: %w", err)
	}

	// 只处理 main/master 分支
	if !strings.HasSuffix(payload.Ref, "/main") && !strings.HasSuffix(payload.Ref, "/master") {
		return nil, fmt.Errorf("ignoring push to non-main branch: %s", payload.Ref)
	}

	service := payload.Project.Name
	if parts := strings.Split(payload.Project.PathWithNamespace, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	commitMsg := ""
	if len(payload.Commits) > 0 {
		commitMsg = payload.Commits[0].Message
	}

	return &protocol.ChangeEvent{
		Source:    protocol.ChangeSourceGitLab,
		EventType: protocol.ChangeTypeGitPush,
		Service:   service,
		Env:       inferEnv(branch),
		Summary:   fmt.Sprintf("Push to %s: %s", branch, commitMsg),
		StartedAt: time.Now(),
		RiskLevel: protocol.ChangeRiskLow,
		DedupeKey: gitlabDedupeKey(deliveryID, "push", payload.Project.PathWithNamespace, payload.After),
		Before: map[string]any{
			"commit": payload.Before[:minLen(8, len(payload.Before))],
		},
		After: map[string]any{
			"commit": payload.After[:minLen(8, len(payload.After))],
			"branch": branch,
		},
		RawPayload: map[string]any{
			"platform": "gitlab",
			"event":    "push",
			"project":  payload.Project.PathWithNamespace,
		},
	}, nil
}

func (a *GitLabAdapter) parseGeneric(body []byte, eventType, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse gitlab generic: %w", err)
	}

	service := "unknown"
	if project, ok := payload["project"].(map[string]any); ok {
		if name, ok := project["name"].(string); ok {
			service = name
		}
	}

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitLab,
		EventType:  protocol.ChangeTypePipeline,
		Service:    service,
		Summary:    fmt.Sprintf("GitLab %s event received", eventType),
		StartedAt:  time.Now(),
		RiskLevel:  protocol.ChangeRiskLow,
		DedupeKey:  gitlabDedupeKey(deliveryID, eventType, service, ""),
		RawPayload: payload,
	}, nil
}

func gitlabDedupeKey(deliveryID, eventName, project, suffix string) string {
	if deliveryID != "" {
		return "gitlab:delivery:" + deliveryID
	}
	parts := []string{"gitlab", eventName, strings.ToLower(project)}
	if suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, ":")
}
