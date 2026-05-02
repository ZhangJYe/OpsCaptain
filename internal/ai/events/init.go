package events

import (
	"context"
	"time"
)

func init() {
	// 启动全局健康度收集器的定期日志聚合
	// 每 5 分钟输出一次工具健康度报告
	GlobalHealthCollector().StartPeriodicReport(context.Background(), 5*time.Minute)
}
