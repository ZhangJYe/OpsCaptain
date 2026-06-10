package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

// RedisLedger 实现 Ledger 接口，把 AIOps 的 task / result / event 状态
// 放到 Redis，让多实例部署下 trace_id / task_id 真正成为全局句柄：
//
//   - 用户提交 /api/ai_ops_runs 落 A 实例 → A 写 Redis
//   - 5 秒后 /api/ai_ops_result?trace_id=... 被 LB 打到 B 实例 → B 直接读 Redis
//   - 不再出现 InMemoryLedger 跨实例 404 的窗口
//
// Redis key 布局：
//
//	String:  ai:task:{task_id}                   JSON TaskEnvelope         (EX = retention)
//	String:  ai:trace:{trace_id}                 task_id (索引)             (EX = retention)
//	String:  ai:result:{task_id}                 JSON TaskResult            (EX = retention)
//	List:    ai:children:{parent_task_id}        [child_task_id, ...]       (EX = retention)
//	ZSet:    ai:events:{trace_id}                (event_json, created_at)   (EX = retention)
//	ZSet:    ai:tasks:index                      (task_id, created_at)      —— 全局 cap 用
//	ZSet:    ai:results:index                    (task_id, created_at)      —— 全局 cap 用
//
// 设计取舍：
//   - tasks / results 用独立 key + EX，过期自然清理；index ZSet 做 max cap 修剪
//   - events 用 per-trace ZSet：EventsByTrace 是热路径，避免全局事件 ZSet 大范围扫描
//   - ListChildren 用 List：插入 O(1)，读取 O(N) 但子任务数有限，可接受
type RedisLedger struct {
	redis      *gredis.Redis
	prefix     string
	retention  time.Duration
	maxTasks   int
	maxResults int
}

// NewRedisLedger 构造一个 Redis-backed ledger。
//   - prefix: 默认 "opscaption:ai:"；多个独立 AIOps 集群可用不同前缀
//   - retention: tasks/results/events 的 TTL；建议 ≥ 24h 便于回放追溯
//   - maxTasks/maxResults: 全局上限（仅 trim index），实际清理还看 TTL
func NewRedisLedger(redis *gredis.Redis, prefix string, retention time.Duration, maxTasks, maxResults int) *RedisLedger {
	if prefix == "" {
		prefix = "opscaption:ai:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	if maxTasks <= 0 {
		maxTasks = 20000
	}
	if maxResults <= 0 {
		maxResults = 20000
	}
	return &RedisLedger{
		redis:      redis,
		prefix:     prefix,
		retention:  retention,
		maxTasks:   maxTasks,
		maxResults: maxResults,
	}
}

// === Ledger 接口实现 ===

func (l *RedisLedger) CreateTask(ctx context.Context, task *protocol.TaskEnvelope) error {
	if l == nil || l.redis == nil {
		return fmt.Errorf("redis ledger not configured")
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("task with empty TaskID")
	}
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	ttlSec := int(l.retention.Seconds())

	// 1. task 主体（如已存在则覆盖，便于 idempotent 重投）
	if _, err := l.redis.Do(ctx, "SET", l.taskKey(task.TaskID), string(data), "EX", ttlSec); err != nil {
		return fmt.Errorf("save task: %w", err)
	}

	// 2. trace_id → task_id 反向索引，加速 TaskByTraceID
	if strings.TrimSpace(task.TraceID) != "" {
		_, _ = l.redis.Do(ctx, "SET", l.traceIndexKey(task.TraceID), task.TaskID, "EX", ttlSec)
	}

	// 3. 全局 index ZSet（按创建时间），用于 cap 修剪
	_, _ = l.redis.Do(ctx, "ZADD", l.taskIndexKey(), task.CreatedAt, task.TaskID)

	// 4. 父子关系（用 List 便于 ListChildren 单次读取）
	if strings.TrimSpace(task.ParentTaskID) != "" {
		childKey := l.childrenKey(task.ParentTaskID)
		_, _ = l.redis.Do(ctx, "RPUSH", childKey, task.TaskID)
		_, _ = l.redis.Do(ctx, "EXPIRE", childKey, ttlSec)
	}

	// 5. cap 修剪（按 ZSET rank，淘汰最早的）
	if l.maxTasks > 0 {
		_, _ = l.redis.Do(ctx, "ZREMRANGEBYRANK", l.taskIndexKey(), 0, -(l.maxTasks + 1))
	}

	return nil
}

func (l *RedisLedger) UpdateTaskStatus(ctx context.Context, taskID string, status protocol.TaskStatus) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	if l == nil || l.redis == nil {
		return fmt.Errorf("redis ledger not configured")
	}
	// 读-改-写。Redis 这里没有原子 patch，可以用 Lua 优化；目前 task 写入频率不高，先简单。
	val, err := l.redis.Do(ctx, "GET", l.taskKey(taskID))
	if err != nil || val == nil || val.IsNil() {
		return nil // 任务已被清理
	}
	var task protocol.TaskEnvelope
	if err := json.Unmarshal([]byte(val.String()), &task); err != nil {
		return fmt.Errorf("unmarshal task: %w", err)
	}
	task.Status = status
	task.UpdatedAt = nowMillis()
	data, err := json.Marshal(&task)
	if err != nil {
		return err
	}
	ttlSec := int(l.retention.Seconds())
	_, err = l.redis.Do(ctx, "SET", l.taskKey(taskID), string(data), "EX", ttlSec)
	return err
}

func (l *RedisLedger) AppendResult(ctx context.Context, taskID string, result *protocol.TaskResult) error {
	if l == nil || l.redis == nil {
		return fmt.Errorf("redis ledger not configured")
	}
	if result == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	ttlSec := int(l.retention.Seconds())
	if _, err := l.redis.Do(ctx, "SET", l.resultKey(taskID), string(data), "EX", ttlSec); err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	_, _ = l.redis.Do(ctx, "ZADD", l.resultIndexKey(), nowMillis(), taskID)
	if l.maxResults > 0 {
		_, _ = l.redis.Do(ctx, "ZREMRANGEBYRANK", l.resultIndexKey(), 0, -(l.maxResults + 1))
	}
	return nil
}

func (l *RedisLedger) AppendEvent(ctx context.Context, event *protocol.TaskEvent) error {
	if l == nil || l.redis == nil {
		return fmt.Errorf("redis ledger not configured")
	}
	if event == nil || strings.TrimSpace(event.TraceID) == "" {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	key := l.eventsKey(event.TraceID)
	ttlSec := int(l.retention.Seconds())
	score := event.CreatedAt
	if score == 0 {
		score = nowMillis()
	}
	// ZADD member=JSON 字符串，score=created_at；EventsByTrace 按 score 升序取。
	// 注意：相同 score+member 会被 Redis 视为重复（这里有意：同事件多次发送幂等）。
	if _, err := l.redis.Do(ctx, "ZADD", key, score, string(data)); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	_, _ = l.redis.Do(ctx, "EXPIRE", key, ttlSec)
	return nil
}

func (l *RedisLedger) EventsByTrace(ctx context.Context, traceID string) ([]*protocol.TaskEvent, error) {
	if l == nil || l.redis == nil {
		return nil, fmt.Errorf("redis ledger not configured")
	}
	if strings.TrimSpace(traceID) == "" {
		return nil, nil
	}
	val, err := l.redis.Do(ctx, "ZRANGE", l.eventsKey(traceID), 0, -1)
	if err != nil {
		return nil, fmt.Errorf("zrange events: %w", err)
	}
	if val == nil {
		return nil, nil
	}
	rawMembers := val.Strings()
	out := make([]*protocol.TaskEvent, 0, len(rawMembers))
	for _, raw := range rawMembers {
		var ev protocol.TaskEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			g.Log().Warningf(ctx, "[redis-ledger] skip malformed event: %v", err)
			continue
		}
		out = append(out, &ev)
	}
	// ZRANGE 已按 score 升序，但保险起见再排一下（兼容旧写入的乱序数据）
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (l *RedisLedger) ListChildren(ctx context.Context, parentTaskID string) ([]*protocol.TaskEnvelope, error) {
	if l == nil || l.redis == nil {
		return nil, fmt.Errorf("redis ledger not configured")
	}
	if strings.TrimSpace(parentTaskID) == "" {
		return nil, nil
	}
	val, err := l.redis.Do(ctx, "LRANGE", l.childrenKey(parentTaskID), 0, -1)
	if err != nil {
		return nil, fmt.Errorf("lrange children: %w", err)
	}
	if val == nil {
		return nil, nil
	}
	childIDs := val.Strings()
	out := make([]*protocol.TaskEnvelope, 0, len(childIDs))
	for _, id := range childIDs {
		t, err := l.getTask(ctx, id)
		if err != nil || t == nil {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (l *RedisLedger) ResultByTaskID(ctx context.Context, taskID string) (*protocol.TaskResult, error) {
	if l == nil || l.redis == nil {
		return nil, fmt.Errorf("redis ledger not configured")
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	val, err := l.redis.Do(ctx, "GET", l.resultKey(taskID))
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}
	if val == nil || val.IsNil() {
		return nil, nil
	}
	var r protocol.TaskResult
	if err := json.Unmarshal([]byte(val.String()), &r); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}
	return &r, nil
}

func (l *RedisLedger) TaskByTraceID(ctx context.Context, traceID string) (*protocol.TaskEnvelope, error) {
	if strings.TrimSpace(traceID) == "" {
		return nil, nil
	}
	if l == nil || l.redis == nil {
		return nil, fmt.Errorf("redis ledger not configured")
	}
	idVal, err := l.redis.Do(ctx, "GET", l.traceIndexKey(traceID))
	if err != nil {
		return nil, fmt.Errorf("get trace index: %w", err)
	}
	if idVal == nil || idVal.IsNil() {
		return nil, nil
	}
	return l.getTask(ctx, idVal.String())
}

// === 私有 helpers ===

func (l *RedisLedger) getTask(ctx context.Context, taskID string) (*protocol.TaskEnvelope, error) {
	val, err := l.redis.Do(ctx, "GET", l.taskKey(taskID))
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if val == nil || val.IsNil() {
		return nil, nil
	}
	var t protocol.TaskEnvelope
	if err := json.Unmarshal([]byte(val.String()), &t); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}
	return &t, nil
}

func (l *RedisLedger) taskKey(id string) string        { return l.prefix + "task:" + id }
func (l *RedisLedger) traceIndexKey(id string) string  { return l.prefix + "trace:" + id }
func (l *RedisLedger) resultKey(id string) string      { return l.prefix + "result:" + id }
func (l *RedisLedger) childrenKey(id string) string    { return l.prefix + "children:" + id }
func (l *RedisLedger) eventsKey(traceID string) string { return l.prefix + "events:" + traceID }
func (l *RedisLedger) taskIndexKey() string            { return l.prefix + "tasks:index" }
func (l *RedisLedger) resultIndexKey() string          { return l.prefix + "results:index" }

// strconvAtoi 留作未来扩展（如 score 解析）
var _ = strconv.Atoi
