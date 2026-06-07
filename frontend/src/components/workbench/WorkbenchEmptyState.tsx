import { motion } from 'framer-motion'
import { Database, MessageSquareText, Search, ShieldCheck } from 'lucide-react'
import { AIOpsPanel } from '../welcome/AIOpsPanel'
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

const workbenchNotes = [
  {
    icon: Search,
    label: 'Context',
    value: '先理解问题和约束',
  },
  {
    icon: Database,
    label: 'Evidence',
    value: '补齐历史、知识库和文件',
  },
  {
    icon: ShieldCheck,
    label: 'Answer',
    value: '给出结论、证据和动作',
  },
]

const contextSteps = [
  '识别服务、时间窗、错误类型和影响面',
  '关联已选能力、会话记忆和上传文档',
  '输出可复核的结论与后续追问建议',
]

export function WorkbenchEmptyState({ onSend, onStartAIOps }: Props) {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col px-4 py-5 sm:py-6">
      <motion.div
        initial={{ opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35 }}
        className="grid gap-6 text-left lg:grid-cols-[minmax(0,1fr)_320px] lg:items-end"
      >
        <div>
          <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-white/50 bg-white/70 px-3 py-1.5 text-[11px] font-medium text-zinc-500 shadow-sm backdrop-blur-sm dark:border-white/10 dark:bg-slate-800/50 dark:text-zinc-400">
            <MessageSquareText size={13} className="text-sky-500" />
            ReAct 问答工作台
          </div>
          <h1 className="max-w-3xl text-[2rem] font-semibold tracking-normal text-zinc-950 dark:text-zinc-50 sm:text-[2.45rem]">
            描述问题，OpsCaption 会先收集上下文再回答。
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-zinc-500 dark:text-zinc-400">
            适合日常问答、知识检索、日志片段分析和文档归纳。事故排障入口保留在左侧模式切换里，问答首页只呈现问答能力。
          </p>
        </div>
        <div className="rounded-2xl border border-white/50 bg-white/55 p-4 shadow-sm shadow-zinc-900/[0.03] backdrop-blur-sm dark:border-white/10 dark:bg-slate-800/35">
          <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-zinc-400 dark:text-zinc-600">Context Loop</div>
          <div className="mt-3 space-y-2.5">
            {contextSteps.map((step, index) => (
              <div key={step} className="flex items-start gap-3">
                <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-sky-500/10 text-[11px] font-semibold text-sky-600 dark:text-sky-400">
                  {index + 1}
                </span>
                <span className="text-sm leading-6 text-zinc-600 dark:text-zinc-300">{step}</span>
              </div>
            ))}
          </div>
        </div>
      </motion.div>

      <div className="mt-6">
        <AIOpsPanel onStartDiagnosis={onStartAIOps} />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: 0.08 }}
        className="mt-6 grid gap-3 sm:grid-cols-3"
      >
        {workbenchNotes.map((note) => (
          <div
            key={note.label}
            className="rounded-xl border border-white/50 bg-white/55 px-4 py-3 shadow-sm shadow-zinc-900/[0.02] backdrop-blur-sm dark:border-white/10 dark:bg-slate-800/35"
          >
            <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-400 dark:text-zinc-600">
              <note.icon size={13} className="text-sky-500" />
              {note.label}
            </div>
            <div className="mt-2 text-sm leading-6 text-zinc-700 dark:text-zinc-300">{note.value}</div>
          </div>
        ))}
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: 0.15 }}
        className="mt-5"
      >
        <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-400 dark:text-zinc-600">Quick Prompts</div>
        <div className="flex flex-wrap gap-2">
          {quickStarters.map((starter) => (
            <button
              key={starter}
              onClick={() => onSend(starter)}
              className="rounded-full border border-white/50 bg-white/55 px-3 py-2 text-xs text-zinc-600 shadow-sm shadow-zinc-900/[0.02] backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:border-sky-400/30 hover:bg-sky-50/80 hover:text-sky-600 dark:border-white/10 dark:bg-slate-800/35 dark:text-zinc-400 dark:hover:border-sky-400/30 dark:hover:bg-sky-500/10 dark:hover:text-sky-400"
            >
              {starter}
            </button>
          ))}
        </div>
      </motion.div>

      <motion.p
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.5, delay: 0.25 }}
        className="mt-6 text-[11px] text-zinc-400 dark:text-zinc-600"
      >
        支持上传 .md .txt .pdf .csv .json .yaml 到知识库
      </motion.p>
    </div>
  )
}
