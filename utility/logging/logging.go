package logging

import (
	"SuperBizAgent/internal/consts"
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
)

const defaultLevel = "INFO"

func Configure(ctx context.Context) error {
	level := resolveLevel(ctx)
	if err := glog.SetConfig(glog.Config{
		Level:               level,
		TimeFormat:          time.RFC3339Nano,
		HeaderPrint:         true,
		LevelPrint:          true,
		StdoutPrint:         true,
		StdoutColorDisabled: true,
		WriterColorEnable:   false,
		CtxKeys: []any{
			consts.CtxKeyTraceID,
			consts.CtxKeySessionID,
			consts.CtxKeyRequestID,
			consts.CtxKeyUserID,
			consts.CtxKeyUserRole,
			consts.CtxKeyApprovalRequestID,
		},
	}); err != nil {
		return err
	}
	glog.SetHandlers(glog.HandlerJson)
	return nil
}

func resolveLevel(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "logging.level")
	if err != nil {
		return parseLevel(defaultLevel)
	}
	level := strings.TrimSpace(v.String())
	if level == "" {
		return parseLevel(defaultLevel)
	}
	return parseLevel(level)
}

func parseLevel(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG", "DEBU":
		return glog.LEVEL_DEBU | glog.LEVEL_INFO | glog.LEVEL_NOTI | glog.LEVEL_WARN | glog.LEVEL_ERRO | glog.LEVEL_CRIT
	case "WARN", "WARNING":
		return glog.LEVEL_WARN | glog.LEVEL_ERRO | glog.LEVEL_CRIT
	case "ERROR", "ERRO":
		return glog.LEVEL_ERRO | glog.LEVEL_CRIT
	default:
		return glog.LEVEL_INFO | glog.LEVEL_NOTI | glog.LEVEL_WARN | glog.LEVEL_ERRO | glog.LEVEL_CRIT
	}
}
