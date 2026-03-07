package middleware

import "github.com/gogf/gf/v2/net/ghttp"

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func ResponseMiddleware(r *ghttp.Request) {
	r.Middleware.Next()

	res := r.GetHandlerResponse()
	err := r.GetError()

	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err != nil {
		r.Response.WriteJson(Response{
			Code:    1,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	r.Response.WriteJson(Response{
		Code:    0,
		Message: "OK",
		Data:    res,
	})
}
