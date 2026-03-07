package main

import (
	// 控制器层：负责 HTTP 入参/出参适配
	controllerChat "StartFromZero/internal/controller/chat"

	// 逻辑层：负责业务逻辑（可替换为真实模型调用）
	logicChat "StartFromZero/internal/logic/chat"

	// 自定义中间件：统一响应结构 + 明确 UTF-8 编码
	"StartFromZero/utility/middleware"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func main() {
	// 1) 创建 GoFrame HTTP Server
	s := g.Server()

	// 2) 组装依赖（依赖注入）
	//    这里先传 nil，表示使用 mock 模式（后续可替换为真实 ModelClient）
	chatService := logicChat.New(nil)
	chatController := controllerChat.New(chatService)

	// 3) 注册 API 路由组
	s.Group("/api", func(group *ghttp.RouterGroup) {
		// 使用自定义响应中间件：
		// - 统一输出 {code, message, data}
		// - 设置 Content-Type: application/json; charset=utf-8
		group.Middleware(middleware.ResponseMiddleware)

		// 绑定控制器（会自动根据 g.Meta 注册 /api/chat）
		group.Bind(chatController)
	})

	// 4) 启动服务（端口由配置文件决定，默认 :8000）
	s.Run()
}
