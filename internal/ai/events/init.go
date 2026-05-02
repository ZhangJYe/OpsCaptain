package events

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	globalHCStarted bool
	globalHCMu      sync.Mutex
)

// StartGlobalHealthReporting 启动全局健康度日志聚合
// 必须在服务启动阶段显式调用，不能用包 init
// interval: 聚合间隔；ctx 取消时停止
func StartGlobalHealthReporting(ctx context.Context, interval time.Duration) {
	globalHCMu.Lock()
	defer globalHCMu.Unlock()

	if globalHCStarted {
		g.Log().Warning(ctx, "[events] global health reporting already started, skipping")
		return
	}
	globalHCStarted = true

	GlobalHealthCollector().StartPeriodicReport(ctx, interval)
	g.Log().Infof(ctx, "[events] global health reporting started, interval=%v", interval)
}
