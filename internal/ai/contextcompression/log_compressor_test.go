package contextcompression

import (
	"strings"
	"testing"
)

func TestCompressLog_PreservesErrorLines(t *testing.T) {
	content := `INFO  Starting application
DEBUG Loading config
INFO  Connected to database
ERROR Connection failed: timeout
INFO  Retrying connection
INFO  Connection restored
INFO  Ready to serve`

	result := compressLog(content, "", 1)
	if !strings.Contains(result, "ERROR Connection failed") {
		t.Error("error line should be preserved")
	}
}

func TestCompressLog_PreservesContextWindow(t *testing.T) {
	content := `line1
line2
line3
ERROR something broke
line5
line6
line7`

	result := compressLog(content, "", 1)
	// 上下文窗口: ERROR 的前1行和后1行应被保留
	if !strings.Contains(result, "line3") {
		t.Error("context line before error should be preserved")
	}
	if !strings.Contains(result, "line5") {
		t.Error("context line after error should be preserved")
	}
}

func TestCompressLog_PreservesWarnLines(t *testing.T) {
	content := `INFO  normal operation
INFO  still normal
WARN  high memory usage
INFO  continuing
INFO  all good`

	result := compressLog(content, "", 0)
	if !strings.Contains(result, "WARN  high memory usage") {
		t.Error("warn line should be preserved")
	}
}

func TestCompressLog_PreservesQueryHits(t *testing.T) {
	content := `INFO  processing request
DEBUG validating input
INFO  querying paymentservice for status
INFO  request completed
INFO  logging result`

	result := compressLog(content, "paymentservice", 0)
	if !strings.Contains(result, "paymentservice") {
		t.Error("query-hit line should be preserved")
	}
}

func TestCompressLog_DedupConsecutiveDuplicateLines(t *testing.T) {
	content := `INFO  start
ERROR connection failed
ERROR connection failed
ERROR connection failed
ERROR connection failed
INFO  retrying
INFO  done`

	result := compressLog(content, "", 0)
	// 连续重复的 ERROR 行应该去重
	count := strings.Count(result, "ERROR connection failed")
	if count != 1 {
		t.Errorf("expected 1 duplicate-removed error line, got %d", count)
	}
}

func TestCompressLog_PreservesFirstAndLastLines(t *testing.T) {
	content := `FIRST line
DEBUG something
DEBUG another thing
DEBUG more stuff
DEBUG even more
LAST line`

	result := compressLog(content, "", 0)
	if !strings.Contains(result, "FIRST line") {
		t.Error("first line should always be preserved")
	}
	if !strings.Contains(result, "LAST line") {
		t.Error("last line should always be preserved")
	}
}

func TestCompressLog_ChineseKeywords(t *testing.T) {
	content := `INFO  系统启动
INFO  正常运行
ERROR 数据库连接超时
INFO  重试中
INFO  恢复正常`

	result := compressLog(content, "", 0)
	if !strings.Contains(result, "数据库连接超时") {
		t.Error("Chinese error keyword should be detected")
	}
}

func TestCompressLog_KubernetesEventEvidence(t *testing.T) {
	content := `Name: inventory-worker
Status: Pending
Containers:
  inventory-worker:
    Image: registry.local/inventory-worker:v9.9.9
Events:
  Normal Pulling Pulling image registry.local/inventory-worker:v9.9.9
  Warning Failed Failed to pull image registry.local/inventory-worker:v9.9.9: rpc error: code = NotFound desc = manifest unknown
  Warning Failed Error: ErrImagePull
  Normal BackOff Back-off pulling image registry.local/inventory-worker:v9.9.9
  Warning Failed Error: ImagePullBackOff
Related deployment:
  imagePullSecrets: regcred-prod
  previous stable tag: v2.8.4`

	result := compressLog(content, "ImagePullBackOff manifest unknown registry credentials", 1)
	for _, want := range []string{"ImagePullBackOff", "manifest unknown", "v9.9.9", "regcred-prod"} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected Kubernetes event evidence %q to be preserved in:\n%s", want, result)
		}
	}
}

func TestCompressLog_ShortContent(t *testing.T) {
	content := `line1
line2
line3`

	// 3行内容不应被压缩（<=3行直接返回）
	result := compressLog(content, "", 0)
	if result != content {
		t.Error("short content should not be compressed")
	}
}

func TestCompressLog_EmptyContent(t *testing.T) {
	result := compressLog("", "", 0)
	if result != "" {
		t.Error("empty content should remain empty")
	}
}
