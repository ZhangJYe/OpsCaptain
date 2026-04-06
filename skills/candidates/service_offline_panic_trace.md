# service_offline_panic_trace

## 来源

- `docs/告警处理手册.md`

## 场景

服务下线，怀疑是服务 panic 导致 pod 重启。

## 核心思路

1. 在最近 1 小时日志里按 `panic` 搜索
2. 结合日志主题地域和日志主题 ID 缩小范围
3. 根据 panic 内容分析具体 bug

## 为什么适合做 skill

- 触发条件明确
- 查询关键词稳定
- 有固定排障套路
- 后续可以很自然接进 `logs` specialist

## 更适合的运行时归属

- `logs`

## 下一步怎么实现

- 增加日志查询 focus：`panic`, `stack`, `restart`, `pod`
- 输出 panic 证据和可疑模块
- 补一个 `NextActions`：看最近发布、看堆栈、看重启次数
