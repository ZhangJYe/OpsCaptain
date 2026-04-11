package middleware

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/auth"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/metrics"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	corsOrigins     []string
	corsOriginsOnce sync.Once
)

func loadCORSOrigins(ctx context.Context) []string {
	corsOriginsOnce.Do(func() {
		v, err := g.Cfg().Get(ctx, "cors.allowed_origins")
		if err == nil && v.String() != "" {
			corsOrigins = normalizeCORSOrigins(v.Strings())
		}
	})
	return corsOrigins
}

func ResolveAllowedOrigin(ctx context.Context, origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	origins := loadCORSOrigins(ctx)
	return matchAllowedOrigin(origin, origins)
}

func matchAllowedOrigin(origin string, origins []string) (string, bool) {
	if origin == "" || len(origins) == 0 {
		return "", false
	}
	for _, allowed := range origins {
		switch allowed {
		case "*":
			return "*", true
		case origin:
			return origin, true
		}
	}
	return "", false
}

func normalizeCORSOrigins(origins []string) []string {
	if len(origins) == 0 {
		return nil
	}
	out := make([]string, 0, len(origins))
	for _, allowed := range origins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if allowed == "*" {
			out = append(out, allowed)
			continue
		}
		if resolved, ok := common.ResolveOptionalEnv(allowed); ok {
			out = append(out, strings.TrimSpace(resolved))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isSameOriginRequest(r *ghttp.Request, origin string) bool {
	if r == nil || strings.TrimSpace(origin) == "" {
		return false
	}
	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}
	reqScheme, reqHost := requestSchemeAndHost(r)
	if reqScheme == "" || reqHost == "" {
		return false
	}
	return strings.EqualFold(parsedOrigin.Scheme, reqScheme) && sameHostPort(parsedOrigin.Host, reqHost, reqScheme)
}

func requestSchemeAndHost(r *ghttp.Request) (string, string) {
	if r == nil || r.Request == nil {
		return "", ""
	}
	scheme := strings.TrimSpace(r.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}
	return strings.ToLower(scheme), strings.ToLower(host)
}

func sameHostPort(a, b, scheme string) bool {
	hostA, portA := splitHostPortDefault(a, scheme)
	hostB, portB := splitHostPortDefault(b, scheme)
	return hostA != "" && strings.EqualFold(hostA, hostB) && portA == portB
}

func splitHostPortDefault(hostport, scheme string) (string, string) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", ""
	}
	host, port, err := net.SplitHostPort(hostport)
	if err == nil {
		return strings.ToLower(host), normalizePort(port, scheme)
	}
	return strings.ToLower(hostport), normalizePort("", scheme)
}

func normalizePort(port, scheme string) string {
	port = strings.TrimSpace(port)
	if port != "" {
		return port
	}
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https":
		return "443"
	default:
		return "80"
	}
}

func CORSMiddleware(r *ghttp.Request) {
	origin := r.GetHeader("Origin")
	if origin != "" {
		if isSameOriginRequest(r, origin) {
			r.Response.Header().Set("Access-Control-Allow-Origin", origin)
			r.Response.Header().Set("Vary", "Origin")
			r.Response.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			allowedOrigin, allowed := ResolveAllowedOrigin(r.GetCtx(), origin)
			if !allowed {
				r.Response.WriteStatus(http.StatusForbidden)
				return
			}
			r.Response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			r.Response.Header().Set("Vary", "Origin")
			if allowedOrigin != "*" {
				r.Response.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
	}

	r.Response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	r.Response.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Session-ID")
	r.Response.Header().Set("Access-Control-Max-Age", "3600")

	if r.Method == "OPTIONS" {
		r.Response.WriteStatus(http.StatusNoContent)
		return
	}

	r.Middleware.Next()
}

func AuthMiddleware(r *ghttp.Request) {
	ctx := r.GetCtx()

	authEnabled, err := g.Cfg().Get(ctx, "auth.enabled")
	if err != nil || !authEnabled.Bool() {
		r.Middleware.Next()
		return
	}

	authHeader := r.GetHeader("Authorization")
	if authHeader == "" {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJson(Response{Message: "missing authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJson(Response{Message: "invalid authorization format, expected: Bearer <token>"})
		return
	}

	claims, err := auth.ValidateToken(parts[1])
	if err != nil {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJson(Response{Message: "invalid token: " + err.Error()})
		return
	}

	role := auth.NormalizeRole(claims.Role)
	r.SetCtx(context.WithValue(ctx, consts.CtxKeyUserID, claims.Sub))
	r.SetCtx(context.WithValue(r.GetCtx(), consts.CtxKeyUserRole, role))

	if !authorizePathAccess(r.URL.Path, role) {
		r.Response.WriteStatus(http.StatusForbidden)
		r.Response.WriteJson(Response{Message: "forbidden: insufficient role permissions"})
		return
	}

	r.Middleware.Next()
}

func RateLimitMiddleware(r *ghttp.Request) {
	ctx := r.GetCtx()

	clientID := r.GetClientIp()
	if uid, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && uid != "" {
		clientID = uid
	}

	if err := auth.CheckRateLimit(clientID); err != nil {
		remaining := auth.RemainingTokens(clientID)
		r.Response.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		r.Response.WriteStatus(http.StatusTooManyRequests)
		r.Response.WriteJson(Response{Message: err.Error()})
		return
	}

	r.Middleware.Next()
}

func HealthCheckMiddleware(r *ghttp.Request) {
	if r.URL.Path == "/healthz" {
		r.Response.WriteStatus(http.StatusOK)
		r.Response.WriteJson(Response{Message: "ok"})
		return
	}
	if r.URL.Path == "/readyz" {
		r.Response.WriteStatus(http.StatusOK)
		r.Response.WriteJson(Response{Message: "ready"})
		return
	}
	r.Middleware.Next()
}

func ResponseMiddleware(r *ghttp.Request) {
	r.Middleware.Next()

	if strings.Contains(r.Response.Header().Get("Content-Type"), "text/event-stream") {
		return
	}

	var (
		msg string
		res = r.GetHandlerResponse()
		err = r.GetError()
	)
	if err != nil {
		msg = err.Error()
	} else {
		msg = "OK"
	}
	r.Response.WriteJson(Response{
		Message: msg,
		Data:    res,
	})
}

func TracingMiddleware(r *ghttp.Request) {
	ctx := r.GetCtx()
	if ctx == nil {
		ctx = context.Background()
	}

	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Request.Header))
	ctx, span := traceutil.StartSpan(
		ctx,
		"http",
		r.Method+" "+requestPath(r),
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", requestPath(r)),
			attribute.String("http.target", r.RequestURI),
		),
	)
	r.SetCtx(ctx)
	if traceID := traceutil.CurrentTraceID(ctx); traceID != "" {
		r.Response.Header().Set("X-Trace-ID", traceID)
	}

	defer func() {
		status := r.Response.Status
		if status == 0 {
			status = http.StatusOK
		}
		span.SetAttributes(attribute.Int("http.status_code", status))
		if err := r.GetError(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		span.End()
	}()

	r.Middleware.Next()
}

func MetricsMiddleware(r *ghttp.Request) {
	if r.URL != nil && r.URL.Path == "/metrics" {
		r.Middleware.Next()
		return
	}

	started := time.Now()
	r.Middleware.Next()

	status := r.Response.Status
	if status == 0 {
		status = http.StatusOK
	}
	metrics.ObserveHTTPRequest(r.Method, requestPath(r), status, time.Since(started))
}

func requestPath(r *ghttp.Request) string {
	if r == nil || r.URL == nil || r.URL.Path == "" {
		return "unknown"
	}
	return r.URL.Path
}

func authorizePathAccess(path, role string) bool {
	required := auth.RequiredRolesForPath(path)
	if len(required) == 0 {
		return true
	}
	return auth.IsRoleAllowed(role, required...)
}

type Response struct {
	Message string      `json:"message" dc:"消息提示"`
	Data    interface{} `json:"data"    dc:"执行结果"`
}
