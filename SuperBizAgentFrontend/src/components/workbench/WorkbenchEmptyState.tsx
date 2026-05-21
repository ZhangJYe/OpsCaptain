import { motion } from 'framer-motion'
import { ArrowRight, Network, Route } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { AIOpsEngine } from '../../types/chat'
import { getEngineViewModel } from '../../lib/engineViewModel'

interface Props {
  onSend: (query: string) => void
  onStartAIOps: (query: string) => void
  aiOpsEngine: AIOpsEngine
}

const quickStarters = [
  'paymentservice 延迟升高，先看错误率和队列堆积',
  '请分析 checkout path 最近的 timeout 日志',
  '帮我检索支付超时相关 SOP 和历史案例',
  '请给出回滚、限流和验证步骤',
]

const aiopsDraftQuery = `请按一次真实值班排障的方式分析这个问题：

- 先判断影响范围和风险等级
- 再对齐 metrics、logs、knowledge 三路证据
- 最后给出回滚、限流和验证步骤

异常现象：paymentservice p95 延迟升高，错误率开始抬升，checkout path 出现 timeout。`

const ENGINE_ICONS: Record<AIOpsEngine, LucideIcon> = {
  plan_execute_replan: Route,
  gos_engine: Network,
}

export function WorkbenchEmptyState({ onSend, onStartAIOps, aiOpsEngine }: Props) {
  const view = getEngineViewModel(aiOpsEngine)
  const EngineIcon = ENGINE_ICONS[aiOpsEngine]

  return (
    <div className="mx-auto flex max-w-3xl flex-col items-center px-4 py-5 text-center sm:py-6">
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35 }}
        className="mb-4 sm:mb-5"
      >
        <h1 className="text-2xl font-semibold tracking-normal text-zinc-900 dark:text-zinc-100">
          先给我现象，我去收证据。
        </h1>
        <p className="mt-2 text-sm leading-6 text-zinc-500 dark:text-zinc-400">
          直接贴告警、错误日志、服务名、变更信息，或者上传文档。
        </p>
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: 0.08 }}
        className="mb-4 flex flex-wrap justify-center gap-1.5 sm:mb-5 sm:gap-2"
      >
        {quickStarters.map((starter) => (
          <button
            key={starter}
            onClick={() => onSend(starter)}
            className="rounded-full border border-white/40 bg-white/50 px-3 py-1.5 text-xs text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:border-sky-400/30 hover:text-sky-600 hover:shadow-md dark:border-white/10 dark:bg-slate-700/40 dark:text-zinc-400 dark:hover:border-sky-400/30 dark:hover:text-sky-400 sm:px-3.5 sm:py-2"
          >
            {starter}
          </button>
        ))}
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: 0.18 }}
        className="w-full overflow-hidden rounded-xl border border-white/40 bg-white/40 text-left backdrop-blur-sm dark:border-white/5 dark:bg-slate-800/30"
      >
        <div className="flex items-center justify-between border-b border-white/30 px-4 py-2.5 dark:border-white/5">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-zinc-600 dark:text-zinc-400">AIOps Draft</span>
            <span className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${view.sidebar.flowActive}`}>
              <EngineIcon size={10} />
              {view.badge}
            </span>
          </div>
          <button
            onClick={() => onStartAIOps(aiopsDraftQuery)}
            className={`inline-flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-[11px] font-semibold transition-all hover:-translate-y-0.5 hover:shadow-md ${view.action}`}
          >
            {view.cta}
            <ArrowRight size={12} />
          </button>
        </div>
        <div className="px-3 py-3 sm:px-4">
          <div className="flex items-start gap-3">
            <span className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ring-1 ${view.sidebar.icon}`}>
              <EngineIcon size={16} />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold text-zinc-700 dark:text-zinc-200">{view.draft.title}</span>
                <span className="text-[10px] font-medium text-zinc-400 dark:text-zinc-600">{view.trace}</span>
              </div>
              <p className="mt-1 text-[11px] leading-5 text-zinc-500 dark:text-zinc-500">{view.draft.intro}</p>
            </div>
          </div>
          <div className="mt-3 grid grid-cols-3 gap-1.5 sm:gap-2">
            {view.draft.steps.map((step, index) => (
              <div
                key={step.label}
                className="rounded-lg border border-white/40 bg-white/40 px-2 py-2 backdrop-blur-sm dark:border-white/5 dark:bg-slate-800/30 sm:px-3"
              >
                <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:gap-2">
                  <span className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold ring-1 ${view.sidebar.flowActive}`}>
                    {index + 1}
                  </span>
                  <span className="text-[11px] font-semibold text-zinc-700 dark:text-zinc-300">{step.label}</span>
                </div>
                <p className="mt-1 line-clamp-2 text-[10px] leading-4 text-zinc-500 dark:text-zinc-500">{step.detail}</p>
              </div>
            ))}
          </div>
          <p className="mt-2 line-clamp-2 text-[11px] leading-5 text-zinc-400 dark:text-zinc-600">{view.draft.footer}</p>
        </div>
      </motion.div>

      <motion.p
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.5, delay: 0.25 }}
        className="mt-4 text-[11px] text-zinc-400 dark:text-zinc-600"
      >
        支持上传 .md .txt .pdf .csv .json .yaml 到知识库
      </motion.p>
    </div>
  )
}
