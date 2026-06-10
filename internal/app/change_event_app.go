package app

import (
	"SuperBizAgent/internal/ai/changeevent"
	"SuperBizAgent/internal/ai/protocol"
	aiservice "SuperBizAgent/internal/ai/service"
	"context"
	"fmt"
	"net/http"
	"time"
)

// ChangeEventApp 编排变更事件的业务逻辑。
type ChangeEventApp struct {
	bus     *changeevent.ChangeEventBus
	adapter *changeevent.AdapterRegistry
	broker  *changeevent.NotificationBroker
}

// NewChangeEventApp 创建变更事件应用层。
func NewChangeEventApp(bus *changeevent.ChangeEventBus) *ChangeEventApp {
	return NewChangeEventAppWithAdapterConfig(bus, changeevent.AdapterRegistryConfig{JSONEnabled: true})
}

// NewChangeEventAppWithAdapterConfig creates the change-event app with a
// configured webhook adapter registry.
func NewChangeEventAppWithAdapterConfig(bus *changeevent.ChangeEventBus, adapterCfg changeevent.AdapterRegistryConfig) *ChangeEventApp {
	broker := changeevent.NewNotificationBroker()
	if bus != nil {
		bus.Register(broker)
	}
	return &ChangeEventApp{
		bus:     bus,
		adapter: changeevent.NewAdapterRegistryWithConfig(adapterCfg),
		broker:  broker,
	}
}

// NewDisabledChangeEventApp returns an app that reports change events as disabled.
func NewDisabledChangeEventApp() *ChangeEventApp {
	return &ChangeEventApp{adapter: changeevent.NewAdapterRegistry()}
}

func (app *ChangeEventApp) ensureEnabled() error {
	if app == nil || app.bus == nil {
		return fmt.Errorf("change events are disabled")
	}
	return nil
}

// IngestEvent 接收并处理一个变更事件。
func (app *ChangeEventApp) IngestEvent(ctx context.Context, input *ChangeEventInput) (string, bool, error) {
	if err := app.ensureEnabled(); err != nil {
		return "", false, err
	}
	event := inputToChangeEvent(input)
	return app.bus.Ingest(ctx, event)
}

// QueryEvents 结构化查询变更事件。
func (app *ChangeEventApp) QueryEvents(ctx context.Context, filter QueryFilter) ([]*protocol.ChangeEvent, error) {
	if err := app.ensureEnabled(); err != nil {
		return nil, err
	}
	return app.bus.Query(ctx, filter.toDomain())
}

// GetEvent 获取单个变更事件。
func (app *ChangeEventApp) GetEvent(ctx context.Context, eventID string) (*protocol.ChangeEvent, error) {
	if err := app.ensureEnabled(); err != nil {
		return nil, err
	}
	return app.bus.Store().GetByID(ctx, eventID)
}

// RecentByService 返回指定服务最近的变更事件（内存快速路径）。
func (app *ChangeEventApp) RecentByService(service string, since time.Time, limit int) []*protocol.ChangeEvent {
	if app == nil || app.bus == nil {
		return nil
	}
	return app.bus.RecentByService(service, since, limit)
}

// SubscribeFilter 是应用层的订阅过滤参数，避免 controller 直接依赖 domain 层类型。
type SubscribeFilter struct {
	Services []string
	Env      string
}

// QueryFilter 是应用层的查询过滤参数，避免 controller 直接依赖 domain 层类型。
type QueryFilter struct {
	Services  []string
	Env       string
	Cluster   string
	EventType string
	RiskLevel string
	Since     *time.Time
	Until     *time.Time
	Limit     int
}

func (f QueryFilter) toDomain() protocol.ChangeEventFilter {
	return protocol.ChangeEventFilter{
		Services:  f.Services,
		Env:       f.Env,
		Cluster:   f.Cluster,
		EventType: f.EventType,
		RiskLevel: f.RiskLevel,
		Since:     f.Since,
		Until:     f.Until,
		Limit:     f.Limit,
	}
}

func (f SubscribeFilter) toDomain() changeevent.ChangeEventFilter {
	return changeevent.ChangeEventFilter{
		Services: f.Services,
		Env:      f.Env,
	}
}

// Subscribe subscribes to live change events.
func (app *ChangeEventApp) Subscribe(ctx context.Context, filter SubscribeFilter) (string, <-chan *protocol.ChangeEvent, func(), error) {
	if err := app.ensureEnabled(); err != nil {
		return "", nil, nil, err
	}
	if app.broker == nil {
		return "", nil, nil, fmt.Errorf("change event notification broker is not configured")
	}
	id, ch, cancel := app.broker.Subscribe(ctx, filter.toDomain())
	return id, ch, cancel, nil
}

// IngestFromWebhook 从 webhook payload 接入变更事件。
func (app *ChangeEventApp) IngestFromWebhook(ctx context.Context, source string, headers http.Header, body []byte) (string, bool, error) {
	if err := app.ensureEnabled(); err != nil {
		return "", false, err
	}
	adapter, ok := app.adapter.Get(source)
	if !ok {
		return "", false, fmt.Errorf("unsupported webhook source: %s", source)
	}
	event, err := adapter.Parse(ctx, headers, body)
	if err != nil {
		return "", false, fmt.Errorf("parse webhook: %w", err)
	}
	if event.Source == "" {
		event.Source = source
	}
	return app.bus.Ingest(ctx, event)
}

type changeEventAIOpsRunner struct {
	app    *AIOpsApp
	engine string
}

// NewChangeEventAIOpsRunner adapts AIOpsApp to the proactive analyzer runner.
func NewChangeEventAIOpsRunner(aiopsApp *AIOpsApp, engine string) changeevent.AIOpsRunner {
	return &changeEventAIOpsRunner{app: aiopsApp, engine: engine}
}

func (r *changeEventAIOpsRunner) RunAsync(ctx context.Context, query string) (*changeevent.RunInfo, error) {
	if r == nil || r.app == nil {
		return nil, fmt.Errorf("AIOps app is not configured")
	}
	result, err := r.app.HandleAIOpsRuns(ctx, &AIOpsRunsInput{
		Query:  query,
		Engine: r.engine,
	})
	if err != nil {
		return nil, err
	}
	return &changeevent.RunInfo{
		TraceID: result.TraceID,
		TaskID:  result.TaskID,
		Status:  result.Status,
	}, nil
}

// WithAsyncTimeout 实现 changeevent.AsyncTimeoutOverride，
// 把 ProactiveAnalyzer.InspectionTimeout 透传给 RunAIOpsAsync 内部
// 派生的后台 dispatch goroutine（否则被默认 5min 覆盖）。
func (r *changeEventAIOpsRunner) WithAsyncTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return aiservice.WithAsyncTimeoutOverride(ctx, timeout)
}

// ChangeEventInput 是应用层的变更事件输入。
type ChangeEventInput struct {
	Source     string
	EventType  string
	Service    string
	Env        string
	Namespace  string
	Cluster    string
	Summary    string
	Operator   string
	Before     map[string]any
	After      map[string]any
	Diff       string
	StartedAt  time.Time
	RiskLevel  string
	DedupeKey  string
	RawPayload map[string]any
	Metadata   map[string]any
}

func inputToChangeEvent(input *ChangeEventInput) *protocol.ChangeEvent {
	return &protocol.ChangeEvent{
		Source:     input.Source,
		EventType:  input.EventType,
		Service:    input.Service,
		Env:        input.Env,
		Namespace:  input.Namespace,
		Cluster:    input.Cluster,
		Summary:    input.Summary,
		Operator:   input.Operator,
		Before:     input.Before,
		After:      input.After,
		Diff:       input.Diff,
		StartedAt:  input.StartedAt,
		RiskLevel:  input.RiskLevel,
		DedupeKey:  input.DedupeKey,
		RawPayload: input.RawPayload,
		Metadata:   input.Metadata,
	}
}
