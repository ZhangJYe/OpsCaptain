import { motion } from 'framer-motion'
import { GitBranch, ArrowRight } from 'lucide-react'
import type { AIOpsEngine } from '../../types/chat'

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

function aiOpsEngineLabel(engine: AIOpsEngine): string {
  return engine === 'gos_engine' ? 'GoS Belief' : 'Plan-Execute'
}

export function WorkbenchEmptyState({ onSend, onStartAIOps, aiOpsEngine }: Props) {
  return (
    <div className="mx-auto flex max-w-3xl flex-col items-center px-4 py-6 text-center">
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35 }}
        className="mb-5"
      >
        <h1 className="text-[1.75rem] font-semibold tracking-[-0.04em] text-zinc-900 dark:text-zinc-100 sm:text-2xl">
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
        className="mb-5 flex flex-wrap justify-center gap-2"
      >
        {quickStarters.map((starter) => (
          <button
            key={starter}
            onClick={() => onSend(starter)}
            className="rounded-full border border-white/40 bg-white/50 px-3.5 py-2 text-xs text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:border-sky-400/30 hover:text-sky-600 hover:shadow-md dark:border-white/10 dark:bg-slate-700/40 dark:text-zinc-400 dark:hover:border-sky-400/30 dark:hover:text-sky-400"
          >
            {starter}
          </button>
        ))}
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: 0.18 }}
        className="w-full overflow-hidden rounded-xl border border-white/40 bg-white/40 backdrop-blur-sm dark:border-white/5 dark:bg-slate-800/30"
      >
        <div className="flex items-center justify-between border-b border-white/30 px-4 py-2.5 dark:border-white/5">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-zinc-600 dark:text-zinc-400">AIOps Draft</span>
            <span className="inline-flex items-center gap-1 rounded-md border border-white/40 bg-white/30 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-500">
              <GitBranch size={10} />
              {aiOpsEngineLabel(aiOpsEngine)}
            </span>
          </div>
          <button
            onClick={() => onStartAIOps(aiopsDraftQuery)}
            className="inline-flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-[11px] font-semibold text-sky-600 transition-all hover:-translate-y-0.5 hover:bg-white/60 hover:shadow-md dark:text-sky-400 dark:hover:bg-slate-700/40"
          >
            直接开始
            <ArrowRight size={12} />
          </button>
        </div>
        <pre className="overflow-x-auto whitespace-pre-wrap px-4 py-3 text-[11px] leading-5 text-zinc-500 dark:text-zinc-500">
{aiopsDraftQuery}
        </pre>
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
