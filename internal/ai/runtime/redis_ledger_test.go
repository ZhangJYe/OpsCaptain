package runtime

import (
	"context"
	"testing"
	"time"
)

func TestNewRedisLedger_Defaults(t *testing.T) {
	l := NewRedisLedger(nil, "", 0, 0, 0)
	if l.prefix != "opscaption:ai:" {
		t.Fatalf("expected default prefix, got %s", l.prefix)
	}
	if l.retention != 24*time.Hour {
		t.Fatalf("expected default retention=24h, got %s", l.retention)
	}
	if l.maxTasks != 20000 || l.maxResults != 20000 {
		t.Fatalf("expected default caps 20000")
	}
}

func TestNewRedisLedger_PrefixNormalize(t *testing.T) {
	l := NewRedisLedger(nil, "myns", time.Hour, 100, 100)
	if l.prefix != "myns:" {
		t.Fatalf("expected colon suffix appended, got %s", l.prefix)
	}
	l2 := NewRedisLedger(nil, "withcolon:", time.Hour, 100, 100)
	if l2.prefix != "withcolon:" {
		t.Fatalf("expected no double colon, got %s", l2.prefix)
	}
}

func TestRedisLedger_NilSafe(t *testing.T) {
	var l *RedisLedger
	if err := l.CreateTask(context.Background(), nil); err == nil {
		t.Fatal("nil ledger should error on CreateTask")
	}
	if _, err := l.EventsByTrace(context.Background(), "x"); err == nil {
		t.Fatal("nil ledger should error on EventsByTrace")
	}
}

func TestRedisLedger_EmptyInputs(t *testing.T) {
	// 非 nil ledger 但 redis 也为 nil → 各方法应早返回（错误或空），不应 panic
	l := NewRedisLedger(nil, "test:", time.Hour, 10, 10)
	if err := l.CreateTask(context.Background(), nil); err == nil {
		t.Fatal("nil task should error")
	}
	if err := l.UpdateTaskStatus(context.Background(), "", "running"); err != nil {
		t.Fatalf("empty task id should noop, got %v", err)
	}
	if v, err := l.TaskByTraceID(context.Background(), ""); v != nil || err != nil {
		t.Fatal("empty trace_id should return nil,nil")
	}
}

func TestRedisLedger_KeyLayout(t *testing.T) {
	l := NewRedisLedger(nil, "ns:", time.Hour, 10, 10)
	if got := l.taskKey("t1"); got != "ns:task:t1" {
		t.Fatalf("taskKey wrong: %s", got)
	}
	if got := l.traceIndexKey("tr1"); got != "ns:trace:tr1" {
		t.Fatalf("traceIndexKey wrong: %s", got)
	}
	if got := l.resultKey("t1"); got != "ns:result:t1" {
		t.Fatalf("resultKey wrong: %s", got)
	}
	if got := l.childrenKey("p1"); got != "ns:children:p1" {
		t.Fatalf("childrenKey wrong: %s", got)
	}
	if got := l.eventsKey("tr1"); got != "ns:events:tr1" {
		t.Fatalf("eventsKey wrong: %s", got)
	}
}
