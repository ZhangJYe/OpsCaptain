# etcd 快照备份与灾难恢复

## 适用场景

- Kubernetes 控制面异常，需要确认 etcd 可恢复性
- etcd 成员损坏或 quorum 丢失
- 需要建立“备份不是拍脑袋”的恢复 SOP

## 先记住 3 个结论

1. etcd 要定期做快照，不要等出事再想起备份
2. 恢复时不要把旧成员身份直接硬拼回去
3. Kubernetes 场景下，恢复到旧 revision 后要特别注意 informer / watch 缓存失真

## 快照怎么做

官方建议使用：

```bash
etcdctl snapshot save snapshot.db
```

做完后，可以检查快照状态：

```bash
etcdutl snapshot status snapshot.db -w table
```

你至少应该知道：

- hash
- revision
- total keys
- total size

## 恢复的核心原则

### 1. 恢复会创建新的数据目录

官方文档强调：`etcdutl snapshot restore` 会创建新的 data dir。

这意味着恢复不是“把旧目录原地修一修”，而是基于快照重建逻辑上的新集群。

### 2. 恢复后成员会失去旧身份

恢复时会覆盖部分快照元数据，例如：

- member ID
- cluster ID

这样做是为了避免恢复出来的新成员误加入旧集群。

### 3. 恢复时最好显式指定新成员信息

尤其在 quorum 丢失的情况下，应该明确新集群成员与 token，而不是模糊地沿用旧状态。

## Kubernetes 场景最关键的一点：revision bump

官方文档特别提醒：

- 恢复到旧 snapshot 后，revision 可能回退
- Kubernetes controller / operator 常依赖 watch 和本地缓存
- 如果 revision 回退，缓存可能不会被正确刷新

所以在 Kubernetes 场景里，官方强烈建议使用：

```bash
etcdutl snapshot restore snapshot.db \
  --bump-revision 1000000000 \
  --mark-compacted \
  --data-dir output-dir
```

你可以把它理解成：

- `--bump-revision`：避免恢复后 revision 倒退
- `--mark-compacted`：让旧 watch 失效，逼控制器重建缓存

## 完整恢复时还要注意什么

### 快照完整性

如果快照是 `etcdctl snapshot save` 生成的，会带完整性 hash。
如果只是直接从 data dir 拷文件，则可能需要 `--skip-hash-check`，这说明你的快照可信度更差。

### 新集群成员

官方恢复示例会明确写：

- `--name`
- `--initial-cluster`
- `--initial-cluster-token`
- `--initial-advertise-peer-urls`

这说明恢复不是只有一条命令，而是恢复后的集群拓扑也要一起定义。

## 面试怎么讲

如果面试官问“你会不会做 etcd 恢复”，你不要只回答“会 snapshot restore”。

更完整的说法是：

> 我知道 etcd 恢复不仅是回放 snapshot。  
> 在 Kubernetes 场景里，还要考虑 revision 回退导致 informer cache 失真，所以恢复时应该结合 `--bump-revision` 和 `--mark-compacted`。  
> 同时恢复出来的是新的逻辑集群，需要重新定义成员身份和数据目录。

## 来源

- Source URL: https://etcd.io/docs/v3.7/op-guide/recovery/
