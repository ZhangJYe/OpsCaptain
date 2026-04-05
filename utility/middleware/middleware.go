package middleware

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/auth"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

var (
	corsOrigins     []string
	corsOriginsOnce sync.Once
)

func loadCORSOrigins(ctx context.Context) []string {
	corsOriginsOnce.Do(func() {
		v, err := g.Cfg().Get(ctx, "cors.allowed_origins")
		if err == nil && v.String() != "" {
			corsOrigins = v.Strings()
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

func CORSMiddleware(r *ghttp.Request) {
	origin := r.GetHeader("Origin")
	if origin != "" {
		allowedOrigin, allowed := ResolveAllowedOrigin(r.GetCtx(), origin)
		if !allowed {
			r.Response.WriteStatus(http.StatusForbidden)
			return
		}
		r.Response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		r.Response.Header().Set("Vary", "Origin")
	}

	r.Response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	r.Response.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Session-ID")
	r.Response.Header().Set("Access-Control-Allow-Credentials", "true")
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

	r.SetCtx(context.WithValue(ctx, consts.CtxKeyUserID, claims.Sub))
	r.SetCtx(context.WithValue(r.GetCtx(), consts.CtxKeyUserRole, claims.Role))

	r.Middleware.Next()
}

func RateLimitMiddleware(r *ghttp.Request) {
	ctx := r.GetCtx()

	clientID := r.GetClientIp()
	if uid, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && uid != "" {
		clientID = uid
	}

	if err := auth.CheckRateLimit(clientID); err != nil {
		remaining := auth.GetRateLimiter().RemainingTokens(clientID)
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

type Response struct {
	Message string      `json:"message" dc:"消息提示"`
	Data    interface{} `json:"data"    dc:"执行结果"`
}
