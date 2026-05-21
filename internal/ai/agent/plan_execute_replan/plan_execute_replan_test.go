package plan_execute_replan

import (
	"strings"
	"testing"

	einoschema "github.com/cloudwego/eino/schema"
)

func TestIsAnalysisMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  *einoschema.Message
		want bool
	}{
		{
			name: "assistant with analysis content",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: "## 分析结论\nCPU 资源耗尽导致服务超时，建议扩容",
			},
			want: true,
		},
		{
			name: "assistant with tool calls (not analysis)",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: "我将执行查询",
				ToolCalls: []einoschema.ToolCall{
					{Function: einoschema.FunctionCall{Name: "query_logs"}},
				},
			},
			want: false,
		},
		{
			name: "tool message",
			msg: &einoschema.Message{
				Role:    einoschema.Tool,
				Content: `{"success": true, "data": "CPU usage 95%"}`,
			},
			want: false,
		},
		{
			name: "assistant with short content",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: "ok",
			},
			want: false,
		},
		{
			name: "assistant with plan steps (has tool calls)",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: `{"steps":["step1","step2"]}`,
				ToolCalls: []einoschema.ToolCall{
					{Function: einoschema.FunctionCall{Name: "query_logs"}},
				},
			},
			want: false,
		},
		{
			name: "assistant with long analysis (no tool calls)",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: "根据告警信息和性能指标分析，ranking-stream服务已持续异常运行超过180小时，CPU使用率95%，内存使用率80%，建议立即重启服务并排查根本原因。",
			},
			want: true,
		},
		{
			name: "assistant with JSON plan steps",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: `{"steps":["分析消费者配置参数","检查消费者处理逻辑和性能瓶颈","评估消费者数量和分区分配情况"]}`,
			},
			want: false,
		},
		{
			name: "assistant with JSON plan (has plan key)",
			msg: &einoschema.Message{
				Role:    einoschema.Assistant,
				Content: `{"plan":["step1","step2"],"reason":"need more info"}`,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAnalysisMessage(tt.msg)
			if got != tt.want {
				t.Errorf("isAnalysisMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFallbackAnalysisFromDetails(t *testing.T) {
	got := fallbackAnalysisFromDetails([]string{
		"llm_stream_failed",
		"llm_stream_failed",
		"query metrics returned empty result",
	})
	if !strings.Contains(got, "降级报告") {
		t.Fatalf("expected degraded report, got %q", got)
	}
	if strings.Count(got, "llm_stream_failed") != 1 {
		t.Fatalf("expected compacted unique event, got %q", got)
	}
	if !strings.Contains(got, "LLM 流式响应失败") {
		t.Fatalf("expected stream failure judgment, got %q", got)
	}
}

func TestQueryWithFinalReportRequirement(t *testing.T) {
	got := queryWithFinalReportRequirement("分析 paymentservice")
	if !strings.Contains(got, "最后必须输出") {
		t.Fatalf("expected final report requirement, got %q", got)
	}
	if !strings.Contains(got, "分析 paymentservice") {
		t.Fatalf("expected original query to be preserved, got %q", got)
	}
}
