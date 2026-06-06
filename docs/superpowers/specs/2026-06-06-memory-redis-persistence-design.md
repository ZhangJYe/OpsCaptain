# 记忆系统 Redis 持久化设计

> 日期：2026-06-06
> 状态：待评审
> 目标：记忆重启不丢失 + 多实例共享同一持久化源，消除 Redis 后端全量写入问题

---

## 一、背景

当前记忆系统默认纯内存模式，重启丢失所有长期记忆。可选的文件持久化采用全量 JSON 序列化（每次变更重写整个文件），只适合本地开发或单实例兜底，不支持多实例共享。

## 二、接口改造

### 现有接口（全量）

```go
type LongTermMemoryStore interface {
    Load(ctx context.Context) ([]*MemoryEntry, error)
    Save(ctx context.Context, entries []*MemoryEntry) error
}
```

### 新接口（增量 + 批量）

```go
type LongTermMemoryStore interface {
    // 启动时全量加载
    LoadAll(ctx context.Context) ([]*MemoryEntry, error)
    // 批量写入/更新（原子操作）
    SaveEntries(ctx context.Context, entries []*MemoryEntry) error
    // 批量删除（原子操作）
    DeleteEntries(ctx context.Context, ids []string) error
}
```

- `LoadAll`：启动时全量加载，填充内存 map
- `SaveEntries`：批量写入，覆盖 Store + reinforce + retire 冲突记忆等场景
- `DeleteEntries`：批量删除，覆盖 evict + Forget 等场景

### 为什么需要批量

`StoreWithOptions` 的完整写路径：

```
StoreWithOptions
  ├── 内存 map 写入新 entry           → SaveEntries([entry])
  ├── retireConflictingMemories       → SaveEntries([retired entries...])
  └── evictIfNeeded                   → DeleteEntries([evicted ids...])
```

单条接口会导致 retire/evict 时多次 Redis 往返，也容易漏掉同一次写操作里的关联变更。批量接口用于表达一次内存变更产生的 upsert/delete 集合。

## 三、Redis 实现

### Key 设计

| Key | 类型 | 说明 |
|-----|------|------|
| `{project_id}:memory:entry:{id}` | String (JSON) | 单条记忆 |
| `{project_id}:memory:ids` | Set | 所有记忆 ID 索引 |

`project_id` 复用现有 `memory.project_id`，不新增 key prefix 配置。

### 原子性保证

使用 Redis `MULTI/EXEC` 保证每批操作原子：

```
SaveEntries(entries):
  MULTI
    FOR each entry:
      SET entry:{id} = JSON(entry)
    SADD ids {id1} {id2} ...
  EXEC

DeleteEntries(ids):
  MULTI
    FOR each id:
      DEL entry:{id}
    SREM ids {id1} {id2} ...
  EXEC
```

### 多实例共享边界

每个实例启动时通过 `LoadAll` 从 Redis 加载同一份长期记忆，运行中仍以本地内存 map 作为热缓存。该方案保证多实例共享同一持久化源，但不承诺运行期实时同步。

如果后续需要实时同步，再单独设计 Redis Pub/Sub 或版本轮询。本阶段不实现，避免引入额外复杂度。

### LoadAll 容错

启动时 `SMEMBERS` 拿到 ID 列表后，批量 `GET` 对应 entry。单条失败（数据损坏/缺失）跳过并打 warning 日志，不阻塞启动。

### 序列化

`json.Marshal`/`json.Unmarshal`，与现有格式兼容。Redis 不设 TTL（记忆靠 relevance 衰减淘汰）。

## 四、LongTermMemory 内部改造

### persistLocked 改造（不删除）

保留 `persistLocked` 作为统一持久化出口，但改为 delta 语义：

```go
// 改前：全量序列化
func (ltm *LongTermMemory) persistLocked(ctx context.Context) {
    if ltm.store != nil {
        entries := make([]*MemoryEntry, 0, len(ltm.entries))
        for _, e := range ltm.entries { entries = append(entries, e) }
        ltm.store.Save(ctx, entries)  // 全量写
    }
}

// 改后：delta 持久化
type pendingChanges struct {
    upserts []*MemoryEntry
    deletes []string
}

func (ltm *LongTermMemory) persistChangesLocked(ctx context.Context, changes pendingChanges) {
    if ltm.store == nil { return }
    if len(changes.upserts) > 0 {
        ltm.store.SaveEntries(ctx, changes.upserts)
    }
    if len(changes.deletes) > 0 {
        ltm.store.DeleteEntries(ctx, changes.deletes)
    }
}
```

每个写路径收集 delta，最后统一调用 `persistChangesLocked`：

```
StoreWithOptions:
  changes.upserts = append(changes.upserts, newOrReinforcedEntry)
  changes.upserts = append(changes.upserts, retiredEntries...)
  changes.deletes = append(changes.deletes, evictedIDs...)
  persistChangesLocked(ctx, changes)

Delete:
  changes.deletes = append(changes.deletes, id)
  persistChangesLocked(ctx, changes)

Disable/Promote:
  changes.upserts = append(changes.upserts, updatedEntry)
  persistChangesLocked(ctx, changes)

Forget:
  changes.deletes = append(changes.deletes, removedIDs...)
  persistChangesLocked(ctx, changes)

RetrieveScoped 非 ReadOnly:
  changes.upserts = append(changes.upserts, accessedEntries...)
  persistChangesLocked(ctx, changes)
```

### 读路径改造

```
改前：RetrieveScoped → 纯内存 map 扫描
改后：RetrieveScoped → 内存 map 扫描
      └── 仍只扫描本地内存 map
```

`RetrieveScoped` 当前遍历的是 `ltm.entries`，没有已知 ID 时无法触发单条读穿透。因此本阶段不加入 `LoadEntry`，避免给“实时共享”造成错误预期。

## 五、文件后端兼容

文件后端保留现有 JSON array 格式，不引入 JSONL、tombstone 或 compaction。

为了适配新接口，file store 内部可以继续全量写：
- `LoadAll`：读取现有 JSON array
- `SaveEntries`：加载当前文件内容，按 ID merge 后全量写回
- `DeleteEntries`：加载当前文件内容，删除指定 ID 后全量写回

文件后端只作为本地开发或单实例兜底，Redis 后端承担生产持久化和共享场景。

## 六、配置

```yaml
memory:
  store_backend: "redis"    # redis / file / 空=纯内存
```

### 初始化逻辑

```
switch store_backend:
  "redis" → newRedisMemoryStore(g.Redis()) → LoadAll 填充内存
  "file"  → newFileMemoryStore(path)       → LoadAll 填充内存
  ""      → nil                            → 纯内存（默认）
```

### 向后兼容

- `store_backend` 为空 → 纯内存模式（现有行为不变）
- 旧配置 `long_term_store_path` 有值 → 自动走 file 后端
- 新配置 `store_backend: "redis"` → 走 Redis

## 七、改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/ai/memory/long_term.go` | 接口改造、persistLocked → persistChangesLocked、delta 收集 |
| `internal/ai/memory/redis_store.go` | 新文件，Redis 实现（MULTI/EXEC 原子写入） |
| `internal/ai/memory/file_store.go` | 从 long_term.go 拆出，保留 JSON array 格式并适配新接口 |
| `internal/ai/memory/long_term_test.go` | 适配新接口 |
| `manifest/config/config.yaml` | 新增 store_backend 配置 |

## 八、不改的部分

- 内存 map 热缓存机制不变
- BM25 检索、Jaccard 去重、relevance 衰减不变
- Session Memory（短期记忆）不变
- Memory Agent 提取逻辑不变
- 不新增配置开关（只有一个 store_backend）
- 不实现多实例运行期实时同步
