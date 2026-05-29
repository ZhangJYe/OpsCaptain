# OpenTelemetry 可观测性与日志关联手册

## 适用场景

- 想解释 traces / metrics / logs 的关系
- 想把日志、指标、链路串起来
- 面试时被问“为什么 observability 不能只看日志”

## 可观测性的核心定义

可观测性的重点不是“采更多数据”，而是：

- 在不知道系统内部细节时，仍然能从外部理解系统行为
- 能处理未知问题，而不只是预设问题

OpenTelemetry 把这件事拆成统一的 telemetry 体系。

## 三类核心信号

- traces
- metrics
- logs

简单理解：

- metrics 适合发现趋势和告警
- traces 适合还原一次请求的跨服务路径
- logs 适合保留具体上下文和错误细节

## 为什么日志不能单独解决所有问题

日志本身通常缺少完整上下文。  
如果日志里没有 trace id、span id、resource 信息，它只能告诉你“发生了什么”，很难高效回答“这次请求跨了哪些服务、是哪个依赖慢、为什么只有部分用户出问题”。

## 日志关联的 3 个维度

1. 时间维度
2. 执行上下文维度（trace context）
3. 资源维度（resource context）

当日志带上 TraceId / SpanId / Resource 后，你才能把：

- 一条日志
- 一段 trace
- 一组指标

真正关联起来。

## 对你这个项目的价值

你项目现在有：

- `logs` specialist
- `metrics` specialist
- `knowledge` specialist

如果后面你想把它做得更像真正的 AIOps，OpenTelemetry 的知识非常关键，因为它决定了：

- 日志如何和 trace 关联
- trace 如何解释“跨服务慢在哪”
- 指标如何支撑 SLI / SLO / 告警

## 面试时可以怎么讲

“我没有把 observability 只理解成日志检索。我专门补了 OpenTelemetry 的可观测性基础和日志关联知识，因为真正的 AIOps 需要把 traces、metrics、logs 放到一个统一上下文里看，而不是孤立查日志。”

## 来源

- OpenTelemetry: `Concepts`
  - https://opentelemetry.io/docs/concepts/
- OpenTelemetry: `Observability primer`
  - https://opentelemetry.io/docs/concepts/observability-primer/
- OpenTelemetry: `Logging / Log Correlation`
  - https://opentelemetry.io/docs/specs/otel/logs/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
