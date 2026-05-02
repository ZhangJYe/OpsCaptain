package main

import (
	"SuperBizAgent/internal/ai/events"
	"SuperBizAgent/internal/ai/models"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/controller/chat"
	"SuperBizAgent/utility/auth"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/health"
	"SuperBizAgent/utility/logging"
	"SuperBizAgent/utility/metrics"
	"SuperBizAgent/utility/middleware"
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
		panic(err)
	}
	if err := aiservice.ValidateMemoryExtractionPipelineConfig(ctx); err != nil {
		panic(err)
	}
	if err := aiservice.ValidateChatTaskPipelineConfig(ctx); err != nil {
		panic(err)
	}

	authEnabled, _ := g.Cfg().Get(ctx, "auth.enabled")
	if authEnabled.Bool() {
		if err := auth.ValidateConfig(); err != nil {
			panic(err)
		}
	}

	fileDir, err := g.Cfg().Get(ctx, "file_dir")
	if err != nil {
		panic(err)
	}
	common.FileDir = fileDir.String()

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
		group.Bind(chat.NewV1())
	})

	if err := s.Start(); err != nil {
		panic(err)
	}

	waitForShutdown(ctx, s, &shuttingDown, pprofServer, traceShutdown, memoryPipelineShutdown, chatTaskPipelineShutdown, healthReportingShutdown)
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
