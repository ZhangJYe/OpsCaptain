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

// GitHubAdapter 解析 GitHub Actions / GitHub webhook payload。
// 支持的事件类型：
//   - workflow_run (GitHub Actions 部署完成)
//   - deployment (GitHub Deployments API)
//   - push (代码推送)
type GitHubAdapter struct {
	secret string
}

func NewGitHubAdapter(secret ...string) *GitHubAdapter {
	a := &GitHubAdapter{}
	if len(secret) > 0 {
		a.secret = strings.TrimSpace(secret[0])
	}
	return a
}

func (a *GitHubAdapter) Name() string {
	return "github"
}

func (a *GitHubAdapter) Parse(ctx context.Context, headers http.Header, body []byte) (*protocol.ChangeEvent, error) {
	if err := verifyHMACSHA256Signature(a.secret, headers.Get("X-Hub-Signature-256"), body); err != nil {
		return nil, err
	}
	// 根据 X-GitHub-Event header 判断事件类型
	eventType := headers.Get("X-GitHub-Event")
	deliveryID := strings.TrimSpace(headers.Get("X-GitHub-Delivery"))

	switch eventType {
	case "workflow_run":
		return a.parseWorkflowRun(body, deliveryID)
	case "deployment":
		return a.parseDeployment(body, deliveryID)
	case "push":
		return a.parsePush(body, deliveryID)
	default:
		// 尝试通用解析
		return a.parseGeneric(body, eventType, deliveryID)
	}
}

// githubWorkflowRun 是 GitHub Actions workflow_run 事件的结构。
type githubWorkflowRun struct {
	Action      string `json:"action"`
	WorkflowRun struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		HeadBranch   string `json:"head_branch"`
		HeadSHA      string `json:"head_sha"`
		Status       string `json:"status"`
		Conclusion   string `json:"conclusion"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		RunStartedAt string `json:"run_started_at"`
		Repository   struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		HeadCommit struct {
			Message string `json:"message"`
		} `json:"head_commit"`
	} `json:"workflow_run"`
}

func (a *GitHubAdapter) parseWorkflowRun(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload githubWorkflowRun
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse github workflow_run: %w", err)
	}

	run := payload.WorkflowRun
	if run.Status != "completed" {
		return nil, fmt.Errorf("workflow not completed, status=%s", run.Status)
	}

	// 从仓库名提取服务名（取最后一段）
	repo := run.Repository.FullName
	service := repo
	if parts := strings.Split(repo, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	// 从 workflow name 推断事件类型。普通 CI pipeline 不等同于线上 deploy。
	eventType := protocol.ChangeTypePipeline
	lowerName := strings.ToLower(run.Name)
	if strings.Contains(lowerName, "rollback") {
		eventType = protocol.ChangeTypeRollback
	} else if strings.Contains(lowerName, "deploy") || strings.Contains(lowerName, "release") || strings.Contains(lowerName, "rollout") {
		eventType = protocol.ChangeTypeDeploy
	} else if strings.Contains(lowerName, "scale") {
		eventType = protocol.ChangeTypeScale
	}
	env := inferEnv(run.Name + " " + run.HeadBranch)

	summary := fmt.Sprintf("GitHub Actions %s completed: %s", run.Name, run.HeadCommit.Message)
	if run.Conclusion != "success" {
		summary = fmt.Sprintf("GitHub Actions %s %s: %s", run.Name, run.Conclusion, run.HeadCommit.Message)
		if eventType == protocol.ChangeTypeDeploy {
			eventType = protocol.ChangeTypePipeline
		}
	}

	startedAt, _ := time.Parse(time.RFC3339, run.RunStartedAt)
	if startedAt.IsZero() {
		startedAt, _ = time.Parse(time.RFC3339, run.CreatedAt)
	}

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitHub,
		EventType:  eventType,
		Service:    service,
		Env:        env,
		Summary:    summary,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		DedupeKey:  githubDedupeKey(deliveryID, "workflow_run", repo, fmt.Sprintf("%d:%s", run.ID, run.Conclusion)),
		Before: map[string]any{
			"head_sha": run.HeadSHA[:minLen(8, len(run.HeadSHA))],
		},
		After: map[string]any{
			"workflow":   run.Name,
			"branch":     run.HeadBranch,
			"conclusion": run.Conclusion,
		},
		RawPayload: map[string]any{
			"platform":    "github",
			"event":       "workflow_run",
			"workflow_id": run.ID,
			"repo":        repo,
		},
		Metadata: map[string]any{
			"commit_sha": run.HeadSHA,
			"branch":     run.HeadBranch,
			"workflow":   run.Name,
		},
	}, nil
}

// githubDeployment 是 GitHub Deployments API 事件的结构。
type githubDeployment struct {
	Deployment struct {
		ID          int64  `json:"id"`
		Ref         string `json:"ref"`
		Environment string `json:"environment"`
		Description string `json:"description"`
		CreatedAt   string `json:"created_at"`
		Task        string `json:"task"`
	} `json:"deployment"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (a *GitHubAdapter) parseDeployment(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload githubDeployment
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse github deployment: %w", err)
	}

	dep := payload.Deployment
	repo := payload.Repository.FullName
	service := repo
	if parts := strings.Split(repo, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	env := dep.Environment
	if env == "" {
		env = "unknown"
	} else {
		env = normalizeEnv(env)
	}

	startedAt, _ := time.Parse(time.RFC3339, dep.CreatedAt)

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitHub,
		EventType:  protocol.ChangeTypeDeploy,
		Service:    service,
		Env:        env,
		Summary:    fmt.Sprintf("GitHub Deployment: %s to %s", dep.Ref, env),
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		DedupeKey:  githubDedupeKey(deliveryID, "deployment", repo, fmt.Sprintf("%d:%s", dep.ID, env)),
		After: map[string]any{
			"ref":         dep.Ref,
			"environment": env,
			"task":        dep.Task,
		},
		RawPayload: map[string]any{
			"platform":      "github",
			"event":         "deployment",
			"deployment_id": dep.ID,
			"repo":          repo,
		},
	}, nil
}

// githubPush 是 GitHub push 事件的结构。
type githubPush struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit"`
}

func (a *GitHubAdapter) parsePush(body []byte, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload githubPush
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse github push: %w", err)
	}

	// 只处理 main/master 分支的 push
	ref := payload.Ref
	if !strings.HasSuffix(ref, "/main") && !strings.HasSuffix(ref, "/master") {
		return nil, fmt.Errorf("ignoring push to non-main branch: %s", ref)
	}

	repo := payload.Repository.FullName
	service := repo
	if parts := strings.Split(repo, "/"); len(parts) > 1 {
		service = parts[len(parts)-1]
	}

	branch := strings.TrimPrefix(ref, "refs/heads/")

	return &protocol.ChangeEvent{
		Source:    protocol.ChangeSourceGitHub,
		EventType: protocol.ChangeTypeGitPush,
		Service:   service,
		Env:       inferEnv(branch),
		Summary:   fmt.Sprintf("Push to %s: %s", branch, payload.HeadCommit.Message),
		Operator:  payload.HeadCommit.Author.Name,
		StartedAt: time.Now(),
		RiskLevel: protocol.ChangeRiskLow,
		DedupeKey: githubDedupeKey(deliveryID, "push", repo, payload.After),
		Before: map[string]any{
			"commit": payload.Before[:minLen(8, len(payload.Before))],
		},
		After: map[string]any{
			"commit": payload.After[:minLen(8, len(payload.After))],
			"branch": branch,
		},
		RawPayload: map[string]any{
			"platform": "github",
			"event":    "push",
			"repo":     repo,
		},
	}, nil
}

func (a *GitHubAdapter) parseGeneric(body []byte, eventName, deliveryID string) (*protocol.ChangeEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse github generic: %w", err)
	}

	service := "unknown"
	if repo, ok := payload["repository"].(map[string]any); ok {
		if name, ok := repo["full_name"].(string); ok {
			service = name
			if parts := strings.Split(name, "/"); len(parts) > 1 {
				service = parts[len(parts)-1]
			}
		}
	}

	return &protocol.ChangeEvent{
		Source:     protocol.ChangeSourceGitHub,
		EventType:  protocol.ChangeTypePipeline,
		Service:    service,
		Summary:    "GitHub webhook event received",
		StartedAt:  time.Now(),
		RiskLevel:  protocol.ChangeRiskLow,
		DedupeKey:  githubDedupeKey(deliveryID, eventName, service, ""),
		RawPayload: payload,
	}, nil
}

func githubDedupeKey(deliveryID, eventName, repo, suffix string) string {
	if deliveryID != "" {
		return "github:delivery:" + deliveryID
	}
	parts := []string{"github", eventName, strings.ToLower(repo)}
	if suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, ":")
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
