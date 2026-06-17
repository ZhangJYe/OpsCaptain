package main

import (
	chat_pipeline "SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	"SuperBizAgent/internal/ai/changeevent"
	"SuperBizAgent/internal/ai/events"
	"SuperBizAgent/internal/ai/indexer"
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/retriever"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/app"
	"SuperBizAgent/internal/controller/chat"
	infrafs "SuperBizAgent/internal/infra/filestore"
	inframv "SuperBizAgent/internal/infra/milvus"
	"SuperBizAgent/internal/infra/notifier"
	"SuperBizAgent/internal/infra/rabbitmq"
	"SuperBizAgent/utility/auth"
	"SuperBizAgent/utility/clusterbus"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/health"
	"SuperBizAgent/utility/logging"
	"SuperBizAgent/utility/metrics"
	"SuperBizAgent/utility/middleware"
	"SuperBizAgent/utility/safety"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
)

func main() {
	if err := common.LoadPreferredEnvFile(); err != nil {
		panic(err)
	}
	ctx := gctx.New()

	if err := common.ConfigureRedis(ctx); err != nil {
		panic(err)
	}

	if err := logging.Configure(ctx); err != nil {
		panic(err)
	}

	traceShutdown, err := traceutil.Init(ctx)
	if err != nil {
		panic(err)
	}
	models.SetTokenAuditHooks(aiservice.EnforceTokenLimitFromContext, aiservice.RecordTokenUsageFromContext)

	if err := common.ValidateStartupSecrets(ctx); err != nil {
		if common.RequireStartupModelSecrets(ctx) {
			panic(err)
		}
		g.Log().Warningf(ctx, "AI model configuration is incomplete, AI requests may degrade until configuration is fixed: %v", err)
	}
	if err := aiservice.ValidateMemoryExtractionPipelineConfig(ctx); err != nil {
		panic(err)
	}
	if err := aiservice.ValidateChatTaskPipelineConfig(ctx); err != nil {
		panic(err)
	}

	// 短期对话存储：多实例配置下切到 Redis，跨实例共享对话历史。
	// 默认 in-process（向后兼容单实例）。
	if configBool(ctx, "memory.session_store.redis_enabled", false) {
		ttl := time.Duration(configInt(ctx, "memory.session_store.ttl_seconds", 7200)) * time.Second
		prefix := configString(ctx, "memory.session_store.prefix", "opscaption:session:")
		memory.EnableRedisSessionStore(prefix, ttl)
		g.Log().Infof(ctx, "[memory] session store: redis (prefix=%s, ttl=%s)", prefix, ttl)
	}

	authEnabled, _ := g.Cfg().Get(ctx, "auth.enabled")
	if authEnabled.Bool() {
		if err := auth.ValidateConfig(); err != nil {
			panic(err)
		}
	} else {
		g.Log().Warningf(ctx, "⚠️  AUTH DISABLED: auth.enabled is false. All API endpoints are accessible without authentication. Do NOT use this in production.")
	}

	fileDir, err := g.Cfg().Get(ctx, "file_dir")
	if err != nil {
		panic(err)
	}
	common.FileDir = fileDir.String()

	rag.DefaultIndexingService().SyncBM25Index(ctx)
	rag.SetDefaultVectorStore(inframv.NewMilvusVectorStore(inframv.NewMilvusClient))
	milvusClient, milvusErr := inframv.NewMilvusClient(ctx)
	if milvusErr == nil {
		milvusCfg := inframv.MilvusConfigFromContext(ctx)
		knowledge_index_pipeline.NewIndexerFunc = indexer.NewMilvusIndexerWithConfig(indexer.MilvusIndexerConfig{
			Client:         milvusClient,
			CollectionName: milvusCfg.CollectionName,
			Fields:         inframv.BuildMilvusFields(milvusCfg),
		})
	}
	if milvusErr != nil {
		g.Log().Warningf(ctx, "milvus client init failed, retriever will be unavailable: %v", milvusErr)
	} else {
		rag.NewRetrieverFunc = retriever.NewMilvusRetrieverWithClient(milvusClient)
	}
	safety.ClassifierModelFunc = models.OpenAIForGLMFast
	health.CloseMySQLFunc = tools.CloseMySQL
	health.CloseAllMilvusClientsFunc = inframv.CloseAllClients
	health.MilvusReadyCheckFunc = inframv.PingMilvus
	health.InspectMilvusCollectionFunc = func(ctx context.Context) (health.MilvusCollectionReport, error) {
		r, err := inframv.InspectCollection(ctx)
		return health.MilvusCollectionReport{
			Collection: r.Collection,
			SchemaOK:   r.SchemaOK,
			DocCount:   r.DocCount,
		}, err
	}

	aiservice.NewQueueClientFunc = func(cfg aiservice.QueueClientConfig, logPrefix string, onReconnectFailed func(error)) (aiservice.QueueClient, error) {
		topo := rabbitmq.TopologyConfig{
			URL:               cfg.URL,
			Exchange:          cfg.Exchange,
			Queue:             cfg.Queue,
			RoutingKey:        cfg.RoutingKey,
			RetryQueue:        cfg.RetryQueue,
			RetryRoutingKey:   cfg.RetryRoutingKey,
			DLQ:               cfg.DLQ,
			DLQRoutingKey:     cfg.DLQRoutingKey,
			RetryDelay:        cfg.RetryDelay,
			Prefetch:          cfg.Prefetch,
			ConsumerEnabled:   cfg.ConsumerEnabled,
			ConnectionTimeout: cfg.ConnectionTimeout,
		}
		client := rabbitmq.NewClient(topo, logPrefix)
		client.OnReconnectFailed = onReconnectFailed
		if err := client.Connect(); err != nil {
			return nil, err
		}
		return client, nil
	}
	aiservice.ResolveQueueStringFunc = rabbitmq.ResolveRabbitMQString
	aiservice.SleepReconnectFunc = rabbitmq.SleepReconnect
	aiservice.NewTTLSetFunc = func(ttl time.Duration, maxEntries int) aiservice.Deduper {
		return rabbitmq.NewTTLSet(ttl, maxEntries)
	}
	aiservice.NewRedisDeduperFunc = func(redis interface{}, prefix string, ttl time.Duration) aiservice.Deduper {
		return rabbitmq.NewRedisDeduper(redis.(*gredis.Redis), prefix, ttl)
	}
	aiservice.NewCompositeDeduperFunc = func(parts ...aiservice.Deduper) aiservice.Deduper {
		rabbitParts := make([]rabbitmq.Deduper, len(parts))
		for i, p := range parts {
			rabbitParts[i] = p
		}
		return rabbitmq.NewCompositeDeduper(rabbitParts...)
	}

	s := g.Server()
	s.SetGraceful(true)
	s.SetGracefulShutdownTimeout(30)
	s.BindMiddlewareDefault(middleware.TracingMiddleware)
	s.BindMiddlewareDefault(middleware.MetricsMiddleware)

	memoryPipelineShutdown := func(context.Context) error { return nil }
	if shutdownFn, startErr := aiservice.StartMemoryExtractionPipeline(ctx); startErr != nil {
		g.Log().Warningf(ctx, "memory extraction pipeline init failed: %v", startErr)
	} else {
		memoryPipelineShutdown = shutdownFn
	}

	chatApp := app.NewChatApp()
	knowledgeApp := app.NewKnowledgeApp(infrafs.NewLocalUploadStore(common.FileDir))
	aiopsApp := app.NewAIOpsApp()
	app.RegisterChatTaskExecutor(chatApp)

	changeEventApp, changeEventShutdown := configureChangeEvents(ctx, aiopsApp)

	// Initialize user tools system
	userSkillStore := skills.NewFileUserSkillStore(g.Cfg().MustGet(ctx, "user_tools.store_path").String())
	whitelistCfg, _ := g.Cfg().Get(ctx, "user_tools.network_whitelist")
	var whitelist []string
	if whitelistCfg != nil {
		for _, v := range whitelistCfg.Strings() {
			whitelist = append(whitelist, v)
		}
	}
	defaultTimeout, _ := g.Cfg().Get(ctx, "user_tools.default_timeout_ms")
	timeoutMs := defaultTimeout.Int()
	if timeoutMs == 0 {
		timeoutMs = 5000
	}
	dynamicMCPReg, err := tools.NewDynamicMCPRegistry(whitelist, timeoutMs)
	if err != nil {
		g.Log().Warningf(ctx, "init dynamic MCP registry: %v", err)
		dynamicMCPReg, _ = tools.NewDynamicMCPRegistry(nil, timeoutMs)
	}

	// Create user skill loader; domain registries are managed by AIOps runtime, pass nil for now
	customReg, _ := skills.NewRegistry("custom", nil)
	userSkillLoader := skills.NewUserSkillLoader(userSkillStore, dynamicMCPReg, nil, nil, nil, customReg)
	if reloadErr := userSkillLoader.Reload(ctx); reloadErr != nil {
		g.Log().Warningf(ctx, "load user skills: %v", reloadErr)
	}

	mcpToolApp := app.NewMCPToolApp(userSkillStore, dynamicMCPReg)
	userSkillApp := app.NewUserSkillApp(userSkillStore, userSkillLoader)

	// Wire user tool dependencies into chat pipeline for progressive disclosure
	chat_pipeline.SetUserToolDeps(userSkillStore, dynamicMCPReg)

	chatTaskPipelineShutdown := func(context.Context) error { return nil }
	if shutdownFn, startErr := aiservice.StartChatTaskPipeline(ctx); startErr != nil {
		g.Log().Warningf(ctx, "chat task pipeline init failed: %v", startErr)
	} else {
		chatTaskPipelineShutdown = shutdownFn
	}

	healthReportingShutdown := func() {}
	if eventHealthReportingEnabled(ctx) {
		healthReportCtx, cancel := context.WithCancel(ctx)
		healthReportingShutdown = cancel
		events.StartGlobalHealthReporting(healthReportCtx, eventHealthReportInterval(ctx))
	}

	var shuttingDown atomic.Bool
	pprofServer := startPprofServer(ctx)

	s.BindHandler("/healthz", func(r *ghttp.Request) {
		r.Response.WriteStatus(http.StatusOK)
		r.Response.WriteJson(g.Map{"ok": true})
	})
	s.BindHandler("/readyz", func(r *ghttp.Request) {
		report, status := health.BuildReadinessReport(r.GetCtx(), shuttingDown.Load())
		r.Response.WriteStatus(status)
		r.Response.WriteJson(report)
	})
	s.BindHandler("/metrics", func(r *ghttp.Request) {
		metrics.Handler().ServeHTTP(r.Response.RawWriter(), r.Request)
	})

	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.AuthMiddleware)
		group.Middleware(middleware.RateLimitMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1(chatApp, knowledgeApp, aiopsApp, changeEventApp, mcpToolApp, userSkillApp))
	})

	if err := s.Start(); err != nil {
		panic(err)
	}

	waitForShutdown(ctx, s, &shuttingDown, pprofServer, traceShutdown, memoryPipelineShutdown, chatTaskPipelineShutdown, healthReportingShutdown, changeEventShutdown)
}

func configureChangeEvents(ctx context.Context, aiopsApp *app.AIOpsApp) (*app.ChangeEventApp, func()) {
	if !configBool(ctx, "change_events.enabled", true) {
		aiservice.SetChangeEventBus(nil)
		g.Log().Info(ctx, "change events are disabled by config")
		return app.NewDisabledChangeEventApp(), func() {}
	}

	retentionHours := configInt(ctx, "change_events.retention_hours", 720)
	maxEvents := configInt(ctx, "change_events.max_events", 10000)
	storeBackend := strings.ToLower(strings.TrimSpace(configString(ctx, "change_events.store_backend", "redis")))
	var store changeevent.ChangeEventStore
	switch storeBackend {
	case "memory":
		store = changeevent.NewMemoryChangeEventStore()
	case "redis", "":
		store = changeevent.NewRedisChangeEventStore(g.Redis(), "opscaptionai:ce:", retentionHours, maxEvents)
	default:
		g.Log().Warningf(ctx, "unsupported change_events.store_backend=%q, falling back to redis", storeBackend)
		store = changeevent.NewRedisChangeEventStore(g.Redis(), "opscaptionai:ce:", retentionHours, maxEvents)
	}

	changeEventBus := changeevent.NewChangeEventBus(store, configInt(ctx, "change_events.recent_buffer_size", 200))
	if configBool(ctx, "change_events.indexing.enabled", true) {
		changeEventBus.Register(changeevent.NewChangeRAGIndexer(configString(ctx, "change_events.indexing.source_prefix", "change_event")))
	}

	// 分析跟踪器：ProactiveAnalyzer 存 trace_id，FeishuAnalysisNotifier 读取并跟进
	analysisTracker := changeevent.NewAnalysisTracker(500)

	if configBool(ctx, "change_events.proactive.enabled", true) {
		proactiveCfg := changeevent.DefaultProactiveAnalyzerConfig()
		proactiveCfg.Enabled = true
		proactiveCfg.DebounceSeconds = configInt(ctx, "change_events.proactive.debounce_seconds", proactiveCfg.DebounceSeconds)
		proactiveCfg.RequireEnv = configString(ctx, "change_events.proactive.require_env", proactiveCfg.RequireEnv)
		proactiveCfg.RequireRiskLevel = configString(ctx, "change_events.proactive.require_risk_level", proactiveCfg.RequireRiskLevel)
		proactiveCfg.RequireEventTypes = configStringSlice(ctx, "change_events.proactive.require_event_types", proactiveCfg.RequireEventTypes)
		proactiveCfg.InspectionTimeout = configInt(ctx, "change_events.proactive.inspection_timeout_ms", proactiveCfg.InspectionTimeout)
		pa := changeevent.NewProactiveAnalyzer(app.NewChangeEventAIOpsRunner(aiopsApp, ""), proactiveCfg)
		pa.SetTracker(analysisTracker)
		changeEventBus.Register(pa)
	}
	if configBool(ctx, "change_events.notifier.feishu.enabled", false) {
		feishuURL := common.ResolveEnv(configString(ctx, "change_events.notifier.feishu.webhook_url", ""))
		baseURL := common.ResolveEnv(configString(ctx, "change_events.notifier.feishu.base_url", ""))
		if feishuURL != "" {
			changeEventBus.Register(notifier.NewFeishuNotifier(notifier.FeishuNotifierConfig{
				WebhookURL:   feishuURL,
				MinRiskLevel: configString(ctx, "change_events.notifier.feishu.min_risk_level", "medium"),
				Services:     configStringSlice(ctx, "change_events.notifier.feishu.services", nil),
				TimeoutMs:    configInt(ctx, "change_events.notifier.feishu.timeout_ms", 5000),
				BaseURL:      baseURL,
			}))
			// 注册分析结论跟进通知器
			changeEventBus.Register(notifier.NewFeishuAnalysisNotifier(notifier.FeishuAnalysisNotifierConfig{
				Tracker:    analysisTracker,
				Getter:     aiservice.GetAIOpsResult,
				WebhookURL: feishuURL,
				BaseURL:    baseURL,
				TimeoutMs:  5000,
			}))
			g.Log().Info(ctx, "[change_event] feishu notifier + analysis follow-up registered")
		} else {
			g.Log().Warning(ctx, "[change_event] feishu notifier enabled but webhook_url is empty, skipping")
		}
	}
	aiservice.SetChangeEventBus(changeEventBus)

	// SSE 跨实例广播：本实例 ingest 的事件除了 fan-out 给本地 SSE 订阅者，
	// 还 publish 到 clusterbus，其他实例订阅同一 channel 后再 fan-out 给各自的订阅者。
	// 没开启或 Redis 不可达时退化为单实例模式（仅本地 fan-out）。
	var broker *changeevent.NotificationBroker
	if configBool(ctx, "change_events.cluster_broadcast.enabled", true) {
		cbus := clusterbus.New(configString(ctx, "change_events.cluster_broadcast.prefix", "opscaption"))
		channel := configString(ctx, "change_events.cluster_broadcast.channel", "change_events")
		broker = changeevent.NewNotificationBrokerWithCluster(ctx, cbus, channel)
	} else {
		broker = changeevent.NewNotificationBroker()
	}

	// 启动定时清理 goroutine。使用独立可取消 context，保证 waitForShutdown
	// 能在进程退出前优雅停止 cleanup loop —— gctx.New() 包装的是
	// context.Background()，Done 永远不会触发。
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
				deleted, err := store.Cleanup(cleanupCtx, cutoff)
				if err != nil {
					g.Log().Warningf(cleanupCtx, "change event cleanup error: %v", err)
				} else if deleted > 0 {
					g.Log().Infof(cleanupCtx, "change event cleanup: removed %d expired events (before %s)", deleted, cutoff.Format(time.RFC3339))
				}
			}
		}
	}()

	shutdown := func() {
		cancelCleanup()
		select {
		case <-cleanupDone:
		case <-time.After(5 * time.Second):
			g.Log().Warningf(ctx, "change event cleanup goroutine did not stop within 5s")
		}
	}

	return app.NewChangeEventAppWithBroker(changeEventBus, broker, changeEventAdapterConfig(ctx)), shutdown
}

func changeEventAdapterConfig(ctx context.Context) changeevent.AdapterRegistryConfig {
	if !configBool(ctx, "change_events.webhook.enabled", true) {
		return changeevent.AdapterRegistryConfig{}
	}
	return changeevent.AdapterRegistryConfig{
		JSONEnabled: configBool(ctx, "change_events.webhook.json_enabled", false),
		GitHub:      changeEventWebhookSourceConfig(ctx, "github"),
		GitLab:      changeEventWebhookSourceConfig(ctx, "gitlab"),
		ArgoCD:      changeEventWebhookSourceConfig(ctx, "argocd"),
	}
}

func changeEventWebhookSourceConfig(ctx context.Context, source string) changeevent.WebhookAdapterConfig {
	base := "change_events.webhook.sources." + source
	enabled := configBool(ctx, base+".enabled", false)
	secret := configSecret(ctx, base+".secret")
	if enabled && secret == "" {
		g.Log().Warningf(ctx, "change_events webhook source %s is enabled but secret is missing; adapter disabled", source)
		enabled = false
	}
	return changeevent.WebhookAdapterConfig{
		Enabled: enabled,
		Secret:  secret,
	}
}

func configBool(ctx context.Context, key string, fallback bool) bool {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return fallback
	}
	return v.Bool()
}

func configInt(ctx context.Context, key string, fallback int) int {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil || v.Int() <= 0 {
		return fallback
	}
	return v.Int()
}

func configString(ctx context.Context, key string, fallback string) string {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil || strings.TrimSpace(v.String()) == "" {
		return fallback
	}
	return strings.TrimSpace(v.String())
}

func configSecret(ctx context.Context, key string) string {
	raw := configString(ctx, key, "")
	if raw == "" {
		return ""
	}
	resolved, ok := common.ResolveOptionalEnv(raw)
	if !ok || common.LooksLikePlaceholderSecret(resolved) {
		return ""
	}
	return strings.TrimSpace(resolved)
}

func configStringSlice(ctx context.Context, key string, fallback []string) []string {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return fallback
	}
	values := v.Strings()
	if len(values) == 0 {
		return fallback
	}
	return values
}

func waitForShutdown(
	ctx context.Context,
	s *ghttp.Server,
	shuttingDown *atomic.Bool,
	pprofServer *http.Server,
	traceShutdown func(context.Context) error,
	memoryPipelineShutdown func(context.Context) error,
	chatTaskPipelineShutdown func(context.Context) error,
	healthReportingShutdown func(),
	changeEventShutdown func(),
) {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()

	g.Log().Info(ctx, "shutdown signal received")
	shuttingDown.Store(true)
	g.Log().Info(ctx, "server marked unready, waiting for in-flight requests")

	if err := s.Shutdown(); err != nil {
		g.Log().Errorf(ctx, "server shutdown failed: %v", err)
	} else {
		g.Log().Info(ctx, "http server shutdown completed")
	}

	if pprofServer != nil {
		if err := pprofServer.Shutdown(context.Background()); err != nil {
			g.Log().Warningf(ctx, "pprof server shutdown failed: %v", err)
		}
	}

	if changeEventShutdown != nil {
		changeEventShutdown()
	}

	if healthReportingShutdown != nil {
		healthReportingShutdown()
	}

	if memoryPipelineShutdown != nil {
		if err := memoryPipelineShutdown(context.Background()); err != nil {
			g.Log().Warningf(ctx, "memory extraction pipeline shutdown failed: %v", err)
		}
	}
	if chatTaskPipelineShutdown != nil {
		if err := chatTaskPipelineShutdown(context.Background()); err != nil {
			g.Log().Warningf(ctx, "chat task pipeline shutdown failed: %v", err)
		}
	}

	if err := health.CloseResources(ctx); err != nil {
		g.Log().Warningf(ctx, "dependency shutdown completed with errors: %v", err)
	} else {
		g.Log().Info(ctx, "all dependencies closed")
	}
	if err := inframv.CloseAllClients(); err != nil {
		g.Log().Warningf(ctx, "infra milvus clients close failed: %v", err)
	}

	if traceShutdown != nil {
		if err := traceShutdown(context.Background()); err != nil {
			g.Log().Warningf(ctx, "tracing shutdown failed: %v", err)
		}
	}
}

func eventHealthReportingEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "events.health_report_enabled")
	if err != nil {
		return true
	}
	return v.Bool()
}

func eventHealthReportInterval(ctx context.Context) time.Duration {
	const fallback = 5 * time.Minute
	v, err := g.Cfg().Get(ctx, "events.health_report_interval_ms")
	if err != nil || v.Int64() <= 0 {
		return fallback
	}
	return time.Duration(v.Int64()) * time.Millisecond
}

func startPprofServer(ctx context.Context) *http.Server {
	if !pprofEnabled(ctx) {
		return nil
	}

	addr, err := g.Cfg().Get(ctx, "debug.pprof_address")
	pprofAddr := ""
	if err == nil {
		pprofAddr = strings.TrimSpace(addr.String())
	}
	if pprofAddr == "" {
		pprofAddr = "127.0.0.1:6060"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:    pprofAddr,
		Handler: mux,
	}
	go func() {
		g.Log().Infof(ctx, "pprof server listening on %s", pprofAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.Log().Warningf(ctx, "pprof server stopped unexpectedly: %v", err)
		}
	}()
	return srv
}

func pprofEnabled(ctx context.Context) bool {
	if !isProductionEnv() {
		return true
	}
	v, err := g.Cfg().Get(ctx, "debug.pprof_enabled")
	return err == nil && v.Bool()
}

func isProductionEnv() bool {
	for _, value := range []string{
		os.Getenv("APP_ENV"),
		os.Getenv("ENVIRONMENT"),
		os.Getenv("GO_ENV"),
	} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production":
			return true
		}
	}
	return false
}
