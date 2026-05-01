import { motion } from 'framer-motion'
import { Sparkles, Activity, Shield, Zap } from 'lucide-react'

interface Props {
  onSuggestion: (query: string) => void
}

export function AgentGreeting({ onSuggestion }: Props) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5 }}
      className="w-full max-w-2xl"
    >
      {/* Agent avatar + status */}
      <div className="flex items-start gap-4">
        <motion.div
          initial={{ scale: 0.8, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ duration: 0.4, delay: 0.1 }}
          className="relative"
        >
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-accent text-xl font-bold text-white shadow-lg shadow-accent/20">
            AI
          </div>
          <motion.span
            animate={{ opacity: [1, 0.5, 1] }}
            transition={{ duration: 2, repeat: Infinity }}
            className="absolute -bottom-0.5 -right-0.5 flex h-4 w-4 items-center justify-center rounded-full border-2 border-white bg-emerald-400 dark:border-zinc-950"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-white" />
          </motion.span>
        </motion.div>

        <div className="flex-1 pt-1">
          <motion.div
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.4, delay: 0.2 }}
          >
            <div className="flex items-center gap-2">
              <span className="text-lg font-bold text-zinc-900 dark:text-white">OpsCaption</span>
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-400 ring-1 ring-inset ring-emerald-500/20">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
                在线
              </span>
            </div>
            <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
              AI 运维助手 · 多 Agent 协作 · 实时诊断
            </p>
          </motion.div>
        </div>
      </div>

      {/* Greeting message bubble */}
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.35 }}
        className="mt-4 ml-[72px]"
      >
        <div className="rounded-2xl rounded-tl-md border border-zinc-200/80 bg-white/90 px-5 py-4 shadow-sm dark:border-zinc-800/60 dark:bg-zinc-900/70">
          <p className="text-sm leading-7 text-zinc-700 dark:text-zinc-300">
            你好，我是 OpsCaption。<br />
            我可以帮你<span className="font-semibold text-zinc-900 dark:text-white">诊断故障</span>、<span className="font-semibold text-zinc-900 dark:text-white">定位根因</span>、<span className="font-semibold text-zinc-900 dark:text-white">生成处置建议</span>。
          </p>
          <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
            我能并行拉取 <span className="font-medium text-blue-500">Metrics</span>、
            <span className="font-medium text-amber-500">Logs</span>、
            <span className="font-medium text-emerald-500">Knowledge</span> 三路证据，
            自动关联告警和变更事件，检索历史案例和 SOP。
          </p>
        </div>
      </motion.div>

      {/* Status cards */}
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.5 }}
        className="mt-3 ml-[72px] grid gap-2 sm:grid-cols-3"
      >
        <div className="flex items-center gap-2 rounded-xl border border-zinc-200/80 bg-white/70 px-3 py-2.5 dark:border-zinc-800/60 dark:bg-zinc-900/50">
          <Activity size={14} className="text-emerald-400" />
          <span className="text-xs text-zinc-600 dark:text-zinc-400">3 路证据就绪</span>
        </div>
        <div className="flex items-center gap-2 rounded-xl border border-zinc-200/80 bg-white/70 px-3 py-2.5 dark:border-zinc-800/60 dark:bg-zinc-900/50">
          <Shield size={14} className="text-accent" />
          <span className="text-xs text-zinc-600 dark:text-zinc-400">SOP 知识库在线</span>
        </div>
        <div className="flex items-center gap-2 rounded-xl border border-zinc-200/80 bg-white/70 px-3 py-2.5 dark:border-zinc-800/60 dark:bg-zinc-900/50">
          <Zap size={14} className="text-amber-400" />
          <span className="text-xs text-zinc-600 dark:text-zinc-400">快速/流式双模式</span>
        </div>
      </motion.div>

      {/* Suggestions */}
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.65 }}
        className="mt-4 ml-[72px]"
      >
        <p className="mb-2 text-[11px] text-zinc-400 dark:text-zinc-600">你可以这样开始：</p>
        <div className="flex flex-wrap gap-2">
          {[
            { label: '🔍 排查延迟升高', query: '请分析 paymentservice p95 延迟升高的原因，检查错误率、队列堆积和最近变更。' },
            { label: '📋 分析错误日志', query: '请帮我分析最近的错误日志，找出高频异常模式和影响范围。' },
            { label: '🛡️ 生成处置方案', query: 'paymentservice 出现大量超时，请给出回滚和限流方案，标注风险等级。' },
            { label: '📊 检查服务健康', query: '请检查当前各服务的健康状态，识别可能的瓶颈和风险点。' },
          ].map((s) => (
            <button
              key={s.label}
              onClick={() => onSuggestion(s.query)}
              className="rounded-lg border border-zinc-200/80 bg-white/70 px-3 py-2 text-xs text-zinc-600 transition-all hover:border-accent/30 hover:bg-accent/5 hover:text-accent dark:border-zinc-800/60 dark:bg-zinc-900/50 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
            >
              {s.label}
            </button>
          ))}
        </div>
      </motion.div>
    </motion.div>
  )
}
