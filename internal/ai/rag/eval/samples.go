package eval

func SampleCorpus() []RetrievedDoc {
	return []RetrievedDoc{
		{
			ID:      "payment-timeout-sop",
			Title:   "支付超时 SOP",
			Content: "当 payment-service 出现支付超时告警时，先检查上游依赖、数据库连接池、重试队列和最近发布记录。",
		},
		{
			ID:      "prometheus-alert-triage",
			Title:   "Prometheus 告警分诊",
			Content: "收到 Prometheus 告警后，先确认告警规则、触发时间窗口、最近变更和受影响服务，再决定是否升级。",
		},
		{
			ID:      "milvus-connection-playbook",
			Title:   "Milvus 连接排查",
			Content: "如果出现 failed to connect to default database，优先检查 milvus.address、数据库初始化、网络连通性和 collection load 状态。",
		},
		{
			ID:      "mysql-lock-troubleshooting",
			Title:   "MySQL 锁等待排查",
			Content: "遇到 MySQL 锁等待时，先看阻塞链、长事务、慢 SQL 以及热点表更新。",
		},
		{
			ID:      "order-cancel-runbook",
			Title:   "订单取消 Runbook",
			Content: "订单取消异常通常先检查补偿任务、消息积压、库存回滚和订单状态机。",
		},
		{
			ID:      "login-rate-limit-guide",
			Title:   "登录限流指南",
			Content: "登录限流触发时，需要确认网关限流规则、验证码依赖和异常 IP 流量。",
		},
		{
			ID:      "k8s-crashloop-runbook",
			Title:   "Kubernetes CrashLoop 与服务下线排查手册",
			Content: "Pod 反复重启、CrashLoopBackOff、容器启动即崩溃时，先看 Pod 状态和重启次数，再看 describe 事件确认是拉镜像、调度、探针还是容器退出，再看容器日志定位 panic / 配置错误 / 依赖失败。高频原因包括启动参数错误、ConfigMap/Secret 缺失、读写权限不对、探针配置错误。",
		},
		{
			ID:      "k8s-deployment-release",
			Title:   "Kubernetes Deployment 发布与回滚",
			Content: "Deployment 支持滚动发布、版本回退。发布前检查 replicas、strategy、readinessProbe。回滚使用 kubectl rollout undo deployment/<name>，可指定 --to-revision。观察发布状态用 kubectl rollout status。",
		},
		{
			ID:      "prometheus-alerting-design",
			Title:   "Prometheus 告警链路与规则设计手册",
			Content: "Prometheus 告警分两层：Server 评估规则，Alertmanager 负责聚合、抑制、静默和通知。alert rule 核心字段包括 alert、expr、for、keep_firing_for、labels、annotations。for 避免瞬时抖动，annotations 放 runbook link。",
		},
		{
			ID:      "helm-release-rollback",
			Title:   "Helm 发布历史与回滚",
			Content: "Helm 通过 release 管理版本历史。查看历史用 helm history <release>，回滚用 helm rollback <release> <revision>。每次 upgrade 或 rollback 都会递增 revision。失败的 release 状态为 failed 或 pending-upgrade。",
		},
		{
			ID:      "argocd-autosync",
			Title:   "Argo CD 自动同步、自愈和清理",
			Content: "Argo CD 支持 automated sync、self-heal 和 prune。self-heal 会自动回滚手动改动，prune 会删除 Git 仓库中不存在的资源。配置在 Application spec.syncPolicy.automated 中。",
		},
		{
			ID:      "otel-observability",
			Title:   "OpenTelemetry 可观测性与日志关联",
			Content: "OpenTelemetry 提供 traces、metrics、logs 三大信号的统一采集。通过 TraceID/SpanID 关联日志和链路追踪。SDK 自动注入 context propagation，支持 W3C Trace Context。",
		},
		{
			ID:      "ingress-nginx-troubleshooting",
			Title:   "Ingress Nginx 排障与网关迁移",
			Content: "Ingress Nginx 常见问题包括 502/503/504 错误、路由规则冲突、TLS 证书失效、后端服务不健康。排查先看 ingress controller 日志，再检查 upstream 健康状态和 annotation 配置。",
		},
		{
			ID:      "etcd-backup-restore",
			Title:   "etcd 快照备份与恢复",
			Content: "etcd 使用 etcdctl snapshot save 做快照备份，etcdctl snapshot restore 做恢复。备份前确认集群健康状态，恢复时需要停掉 etcd 实例并清理数据目录。定期备份是 Kubernetes 集群灾备的基础。",
		},
		{
			ID:      "mysql-innodb-deadlock",
			Title:   "MySQL InnoDB 死锁与锁等待排查",
			Content: "死锁是两个事务循环等待，锁等待是某事务持锁太久。查死锁用 SHOW ENGINE INNODB STATUS，频繁死锁打开 innodb_print_all_deadlocks。锁等待查 information_schema.INNODB_TRX 和 INNODB_LOCK_WAITS。",
		},
		{
			ID:      "redis-distributed-lock",
			Title:   "Redis 分布式锁边界",
			Content: "Redis 分布式锁使用 SET NX EX 实现，需注意锁续期、主从切换丢锁、锁粒度和业务幂等。Redlock 算法跨多个独立 Redis 实例投票获取锁，但有争议。生产建议配合业务幂等设计。",
		},
		{
			ID:      "sli-slo-guide",
			Title:   "SLI/SLO 实践指南",
			Content: "SLI 是服务质量的量化指标，SLO 是 SLI 的目标值。常见 SLI 包括可用性、延迟、吞吐量、错误率。SLO 设定建议从用户体验出发，避免过高目标浪费成本。Error Budget 用于平衡可靠性与发布速度。",
		},
	}
}

func SampleCases() []EvalCase {
	return []EvalCase{
		{
			ID:          "RAG-01",
			Query:       "支付超时该怎么排查",
			RelevantIDs: []string{"payment-timeout-sop"},
			Notes:       "单文档命中",
		},
		{
			ID:          "RAG-02",
			Query:       "Prometheus 告警先看什么",
			RelevantIDs: []string{"prometheus-alert-triage", "prometheus-alerting-design"},
			Notes:       "双文档命中 - 概要+详细",
		},
		{
			ID:          "RAG-03",
			Query:       "Milvus 连不上 default database 怎么办",
			RelevantIDs: []string{"milvus-connection-playbook"},
			Notes:       "错误文案驱动召回",
		},
		{
			ID:          "RAG-04",
			Query:       "MySQL 锁等待怎么定位",
			RelevantIDs: []string{"mysql-lock-troubleshooting", "mysql-innodb-deadlock"},
			Notes:       "双相关文档 - 概要+详细手册",
		},
		{
			ID:          "RAG-05",
			Query:       "支付告警先怎么分诊",
			RelevantIDs: []string{"payment-timeout-sop", "prometheus-alert-triage"},
			Notes:       "跨域: 支付 + 告警",
		},
		{
			ID:          "RAG-06",
			Query:       "Pod 一直 CrashLoopBackOff 怎么排查",
			RelevantIDs: []string{"k8s-crashloop-runbook"},
			Notes:       "K8s 核心排障场景",
		},
		{
			ID:          "RAG-07",
			Query:       "服务发布后怎么回滚",
			RelevantIDs: []string{"k8s-deployment-release", "helm-release-rollback"},
			Notes:       "跨域: K8s rollout + Helm rollback",
		},
		{
			ID:          "RAG-08",
			Query:       "Argo CD 会自动删除我手动创建的资源吗",
			RelevantIDs: []string{"argocd-autosync"},
			Notes:       "自愈/prune 语义理解",
		},
		{
			ID:          "RAG-09",
			Query:       "怎么用 TraceID 关联日志和链路追踪",
			RelevantIDs: []string{"otel-observability"},
			Notes:       "可观测性场景",
		},
		{
			ID:          "RAG-10",
			Query:       "Ingress 返回 502 怎么排查",
			RelevantIDs: []string{"ingress-nginx-troubleshooting"},
			Notes:       "网关排障",
		},
		{
			ID:          "RAG-11",
			Query:       "etcd 快照怎么做备份和恢复",
			RelevantIDs: []string{"etcd-backup-restore"},
			Notes:       "灾备场景",
		},
		{
			ID:          "RAG-12",
			Query:       "MySQL 死锁和锁等待有什么区别",
			RelevantIDs: []string{"mysql-innodb-deadlock"},
			Notes:       "概念区分类问题",
		},
		{
			ID:          "RAG-13",
			Query:       "Redis 分布式锁主从切换会丢锁吗",
			RelevantIDs: []string{"redis-distributed-lock"},
			Notes:       "边界条件类问题",
		},
		{
			ID:          "RAG-14",
			Query:       "SLO 怎么设定比较合理",
			RelevantIDs: []string{"sli-slo-guide"},
			Notes:       "SRE 实践类问题",
		},
		{
			ID:          "RAG-15",
			Query:       "生产环境数据库事务超时怎么办",
			RelevantIDs: []string{"mysql-innodb-deadlock", "mysql-lock-troubleshooting"},
			Notes:       "模糊表述 - 测语义理解能力",
		},
		{
			ID:          "RAG-16",
			Query:       "容器启动就挂了日志里有 OOM",
			RelevantIDs: []string{"k8s-crashloop-runbook"},
			Notes:       "口语化表述 - 测检索鲁棒性",
		},
		{
			ID:          "RAG-17",
			Query:       "告警规则里 for 字段是干嘛的",
			RelevantIDs: []string{"prometheus-alerting-design"},
			Notes:       "细粒度知识点检索",
		},
		{
			ID:          "RAG-18",
			Query:       "How to rollback a Helm release",
			RelevantIDs: []string{"helm-release-rollback"},
			Notes:       "英文查中文知识库 - 测跨语言召回",
		},
		{
			ID:          "RAG-19",
			Query:       "服务可用性指标怎么定义",
			RelevantIDs: []string{"sli-slo-guide"},
			Notes:       "同义改写 - SLI 不出现在 query 中",
		},
		{
			ID:          "RAG-20",
			Query:       "K8s 发布观察和 Helm 回滚的完整流程",
			RelevantIDs: []string{"k8s-deployment-release", "helm-release-rollback"},
			Notes:       "复合意图 - 同时需要两个文档",
		},
	}
}
