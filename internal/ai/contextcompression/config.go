package contextcompression

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

// CompressionConfig 上下文压缩配置
type CompressionConfig struct {
	Enabled          bool     // 是否启用压缩
	Mode             Mode     // off | audit | optimize
	MinTokens        int      // 小于此 token 数不压缩
	PreserveFirst    int      // JSON: 保留前 N 项
	PreserveLast     int      // JSON: 保留后 M 项
	LogContextLines  int      // 日志: 错误行上下文窗口行数
	SourceTypes      []string // 允许压缩的源类型
}

var (
	defaultConfig = &CompressionConfig{
		Enabled:         false,
		Mode:            ModeOff,
		MinTokens:       300,
		PreserveFirst:   3,
		PreserveLast:    2,
		LogContextLines: 1,
		SourceTypes:     []string{"tool", "rag"},
	}
)

// LoadConfig 从 GoFrame 配置加载压缩配置
func LoadConfig(ctx context.Context) *CompressionConfig {
	cfg := &CompressionConfig{
		Enabled:         defaultConfig.Enabled,
		Mode:            defaultConfig.Mode,
		MinTokens:       defaultConfig.MinTokens,
		PreserveFirst:   defaultConfig.PreserveFirst,
		PreserveLast:    defaultConfig.PreserveLast,
		LogContextLines: defaultConfig.LogContextLines,
		SourceTypes:     defaultConfig.SourceTypes,
	}

	if v, err := g.Cfg().Get(ctx, "context_compression.enabled"); err == nil {
		cfg.Enabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.mode"); err == nil && v.String() != "" {
		cfg.Mode = Mode(v.String())
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.min_tokens"); err == nil && v.Int() > 0 {
		cfg.MinTokens = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.preserve_first"); err == nil && v.Int() > 0 {
		cfg.PreserveFirst = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.preserve_last"); err == nil && v.Int() > 0 {
		cfg.PreserveLast = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.log_context_lines"); err == nil && v.Int() >= 0 {
		cfg.LogContextLines = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "context_compression.source_types"); err == nil {
		arr := v.Strings()
		if len(arr) > 0 {
			cfg.SourceTypes = arr
		}
	}

	return cfg
}

// SourceTypeAllowed 检查源类型是否在允许列表中
func (c *CompressionConfig) SourceTypeAllowed(st SourceType) bool {
	if len(c.SourceTypes) == 0 {
		return true
	}
	for _, allowed := range c.SourceTypes {
		if allowed == string(st) {
			return true
		}
	}
	return false
}
