# api_failure_rate_investigation

## 来源

- `docs/告警处理手册.md`

## 场景

接口失败率过高，怀疑是服务调用异常或下游不可用。

## 核心思路

1. 根据接口名和 `response` 关键词搜索最近 1 小时日志
2. 根据 error 内容分析失败原因

## 为什么适合做 skill

- 告警到查询的映射稳定
- 查询动作固定
- 很适合沉淀为 API 故障排查模板

## 更适合的运行时归属

- `logs`
- 后续也可以和 `metrics` 联动

## 下一步怎么实现

- 增加 focus：接口名、status code、response、error、upstream/downstream
- 输出失败接口、主要错误类型、疑似下游依赖
