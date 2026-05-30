package eval

func SampleCorpus() []RetrievedDoc {
	return []RetrievedDoc{
		{
			ID:      "01_kubernetes_crashloop_runbook",
			Title:   "Kubernetes CrashLoop 与服务下线排查手册",
			Content: "Pod 反复重启、CrashLoopBackOff、容器启动即崩溃时，先看 Pod 状态和重启次数，再看 describe 事件确认是拉镜像、调度、探针还是容器退出，再看容器日志定位 panic / 配置错误 / 依赖失败。高频原因包括启动参数错误、ConfigMap/Secret 缺失、读写权限不对、探针配置错误。",
		},
		{
			ID:      "02_kubernetes_deployment_release_and_rollback",
			Title:   "Kubernetes Deployment 发布与回滚",
			Content: "Deployment 支持滚动发布、版本回退。发布前检查 replicas、strategy、readinessProbe。回滚使用 kubectl rollout undo deployment/<name>，可指定 --to-revision。观察发布状态用 kubectl rollout status。",
		},
		{
			ID:      "03_prometheus_alerting_and_rule_design",
			Title:   "Prometheus 告警链路与规则设计手册",
			Content: "Prometheus 告警分两层：Server 评估规则，Alertmanager 负责聚合、抑制、静默和通知。alert rule 核心字段包括 alert、expr、for、keep_firing_for、labels、annotations。for 避免瞬时抖动，annotations 放 runbook link。收到告警后先确认规则、触发时间窗口、最近变更和受影响服务。",
		},
		{
			ID:      "04_helm_release_history_and_rollback",
			Title:   "Helm 发布历史与回滚",
			Content: "Helm 通过 release 管理版本历史。查看历史用 helm history <release>，回滚用 helm rollback <release> <revision>。每次 upgrade 或 rollback 都会递增 revision。失败的 release 状态为 failed 或 pending-upgrade。",
		},
		{
			ID:      "05_argocd_autosync_prune_selfheal",
			Title:   "Argo CD 自动同步、自愈和清理",
			Content: "Argo CD 支持 automated sync、self-heal 和 prune。self-heal 会自动回滚手动改动，prune 会删除 Git 仓库中不存在的资源。配置在 Application spec.syncPolicy.automated 中。",
		},
		{
			ID:      "06_opentelemetry_observability_and_log_correlation",
			Title:   "OpenTelemetry 可观测性与日志关联",
			Content: "OpenTelemetry 提供 traces、metrics、logs 三大信号的统一采集。通过 TraceID/SpanID 关联日志和链路追踪。SDK 自动注入 context propagation，支持 W3C Trace Context。",
		},
		{
			ID:      "09_ingress_nginx_troubleshooting_and_gateway_transition",
			Title:   "Ingress Nginx 排障与网关迁移",
			Content: "Ingress Nginx 常见问题包括 502/503/504 错误、路由规则冲突、TLS 证书失效、后端服务不健康。排查先看 ingress controller 日志，再检查 upstream 健康状态和 annotation 配置。",
		},
		{
			ID:      "10_etcd_snapshot_backup_and_restore",
			Title:   "etcd 快照备份与恢复",
			Content: "etcd 使用 etcdctl snapshot save 做快照备份，etcdctl snapshot restore 做恢复。备份前确认集群健康状态，恢复时需要停掉 etcd 实例并清理数据目录。定期备份是 Kubernetes 集群灾备的基础。",
		},
		{
			ID:      "11_mysql_innodb_deadlock_and_lock_wait",
			Title:   "MySQL InnoDB 死锁与锁等待排查",
			Content: "死锁是两个事务循环等待，锁等待是某事务持锁太久。查死锁用 SHOW ENGINE INNODB STATUS，频繁死锁打开 innodb_print_all_deadlocks。锁等待查 information_schema.INNODB_TRX 和 INNODB_LOCK_WAITS。热点表、长事务、未命中索引的 UPDATE 是常见根因。",
		},
		{
			ID:      "12_redis_distributed_lock_boundaries",
			Title:   "Redis 分布式锁边界",
			Content: "Redis 分布式锁使用 SET NX EX 实现，需注意锁续期、主从切换丢锁、锁粒度和业务幂等。Redlock 算法跨多个独立 Redis 实例投票获取锁，但有争议。生产建议配合业务幂等设计。",
		},
		{
			ID:      "sli_slo",
			Title:   "SLI/SLO 实践指南",
			Content: "SLI 是服务质量的量化指标，SLO 是 SLI 的目标值。常见 SLI 包括可用性、延迟、吞吐量、错误率。SLO 设定建议从用户体验出发，避免过高目标浪费成本。Error Budget 用于平衡可靠性与发布速度。",
		},
	}
}

func SampleCases() []EvalCase {
	return []EvalCase{
		{
			ID:          "RAG-01",
			Query:       "Prometheus 告警先看什么",
			RelevantIDs: []string{"03_prometheus_alerting_and_rule_design"},
			Notes:       "单文档命中 - 告警分诊",
		},
		{
			ID:          "RAG-02",
			Query:       "MySQL 锁等待怎么定位",
			RelevantIDs: []string{"11_mysql_innodb_deadlock_and_lock_wait"},
			Notes:       "单文档命中 - 锁排障",
		},
		{
			ID:          "RAG-03",
			Query:       "Pod 一直 CrashLoopBackOff 怎么排查",
			RelevantIDs: []string{"01_kubernetes_crashloop_runbook"},
			Notes:       "K8s 核心排障场景",
		},
		{
			ID:          "RAG-04",
			Query:       "服务发布后怎么回滚",
			RelevantIDs: []string{"02_kubernetes_deployment_release_and_rollback", "04_helm_release_history_and_rollback"},
			Notes:       "跨域: K8s rollout + Helm rollback",
		},
		{
			ID:          "RAG-05",
			Query:       "Argo CD 会自动删除我手动创建的资源吗",
			RelevantIDs: []string{"05_argocd_autosync_prune_selfheal"},
			Notes:       "自愈/prune 语义理解",
		},
		{
			ID:          "RAG-06",
			Query:       "怎么用 TraceID 关联日志和链路追踪",
			RelevantIDs: []string{"06_opentelemetry_observability_and_log_correlation"},
			Notes:       "可观测性场景",
		},
		{
			ID:          "RAG-07",
			Query:       "Ingress 返回 502 怎么排查",
			RelevantIDs: []string{"09_ingress_nginx_troubleshooting_and_gateway_transition"},
			Notes:       "网关排障",
		},
		{
			ID:          "RAG-08",
			Query:       "etcd 快照怎么做备份和恢复",
			RelevantIDs: []string{"10_etcd_snapshot_backup_and_restore"},
			Notes:       "灾备场景",
		},
		{
			ID:          "RAG-09",
			Query:       "MySQL 死锁和锁等待有什么区别",
			RelevantIDs: []string{"11_mysql_innodb_deadlock_and_lock_wait"},
			Notes:       "概念区分类问题",
		},
		{
			ID:          "RAG-10",
			Query:       "Redis 分布式锁主从切换会丢锁吗",
			RelevantIDs: []string{"12_redis_distributed_lock_boundaries"},
			Notes:       "边界条件类问题",
		},
		{
			ID:          "RAG-11",
			Query:       "SLO 怎么设定比较合理",
			RelevantIDs: []string{"sli_slo"},
			Notes:       "SRE 实践类问题",
		},
		{
			ID:          "RAG-12",
			Query:       "生产环境数据库事务超时怎么办",
			RelevantIDs: []string{"11_mysql_innodb_deadlock_and_lock_wait"},
			Notes:       "模糊表述 - 测语义理解能力",
		},
		{
			ID:          "RAG-13",
			Query:       "容器启动就挂了日志里有 OOM",
			RelevantIDs: []string{"01_kubernetes_crashloop_runbook"},
			Notes:       "口语化表述 - 测检索鲁棒性",
		},
		{
			ID:          "RAG-14",
			Query:       "告警规则里 for 字段是干嘛的",
			RelevantIDs: []string{"03_prometheus_alerting_and_rule_design"},
			Notes:       "细粒度知识点检索",
		},
		{
			ID:          "RAG-15",
			Query:       "How to rollback a Helm release",
			RelevantIDs: []string{"04_helm_release_history_and_rollback"},
			Notes:       "英文查中文知识库 - 测跨语言召回",
		},
		{
			ID:          "RAG-16",
			Query:       "服务可用性指标怎么定义",
			RelevantIDs: []string{"sli_slo"},
			Notes:       "同义改写 - SLI 不出现在 query 中",
		},
		{
			ID:          "RAG-17",
			Query:       "K8s 发布观察和 Helm 回滚的完整流程",
			RelevantIDs: []string{"02_kubernetes_deployment_release_and_rollback", "04_helm_release_history_and_rollback"},
			Notes:       "复合意图 - 同时需要两个文档",
		},
		{
			ID:          "RAG-18",
			Query:       "支付告警先怎么分诊",
			RelevantIDs: []string{"03_prometheus_alerting_and_rule_design"},
			Notes:       "跨域表述 - 测告警分诊知识召回",
		},
	}
}
