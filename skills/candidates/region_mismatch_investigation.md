# region_mismatch_investigation

## 来源

- `docs/告警处理手册.md`

## 场景

服务地域与资源地域不匹配，怀疑事件错投递到了错误地域。

## 核心思路

1. 搜索 `region mismatch`
2. 汇总调用方和错误地域名

## 为什么适合做 skill

- 场景很垂直，但流程非常稳定
- 输出物也很明确：调用方、错误地域、疑似 MQ 队列

## 更适合的运行时归属

- `logs`
- `knowledge`

## 下一步怎么实现

- logs 侧提取：caller、expected region、actual region
- knowledge 侧补充：队列路由配置、修复 SOP
