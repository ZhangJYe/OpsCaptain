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
			RelevantIDs: []string{"prometheus-alert-triage"},
			Notes:       "单文档命中",
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
			RelevantIDs: []string{"mysql-lock-troubleshooting"},
			Notes:       "数据库排障",
		},
		{
			ID:          "RAG-05",
			Query:       "支付告警先怎么分诊",
			RelevantIDs: []string{"payment-timeout-sop", "prometheus-alert-triage"},
			Notes:       "双相关文档，用来观察 Recall@1 和 Recall@3 的差异",
		},
	}
}
