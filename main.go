package main

import (
	"SuperBizAgent/internal/controller/chat"
	"SuperBizAgent/utility/auth"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/middleware"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
)

func main() {
	if err := common.LoadEnvFile(".env"); err != nil {
		panic(err)
	}
	ctx := gctx.New()

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

	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.HealthCheckMiddleware)
	})

	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.AuthMiddleware)
		group.Middleware(middleware.RateLimitMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})
	s.Run()
}
