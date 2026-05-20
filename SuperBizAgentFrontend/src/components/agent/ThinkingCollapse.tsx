import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, ChevronDown, Brain, Search } from 'lucide-react'
import type { ChatExecutionStep } from '../../types/chat'
import type { DetailItem } from '../workbench/DetailPanel'

export type ThinkingStep = ChatExecutionStep

interface Props {
  steps: ThinkingStep[]
  isStreaming?: boolean
  defaultOpen?: boolean
  onStepClick?: (item: DetailItem) => void
}

function stepToDetail(step: ThinkingStep): DetailItem | null {
  if (!step.detail && (!step.meta || step.meta.length === 0) && step.status !== 'error') return null
  const typeMap: Record<string, DetailItem['type']> = {
    metrics: 'metric', logs: 'log', knowledge: 'knowledge',
    'gos:evidence': 'metric',
  }
  return {
    id: step.id,
    type: typeMap[step.id] || (step.status === 'error' ? 'error' : 'info'),
    title: step.label,
    content: [step.detail, ...(step.meta || [])].filter(Boolean).join('\n\n') || (step.status === 'error' ? `${step.label} 执行失败` : ''),
    meta: step.status === 'error' ? '执行失败' : step.status === 'done' ? '已完成' : '执行中',
  }
}

export function ThinkingCollapse({ steps, isStreaming, defaultOpen, onStepClick }: Props) {
  const [open, setOpen] = useState(defaultOpen ?? Boolean(isStreaming))
  const activeSteps = steps.filter((s) => s.status !== 'pending')

  if (activeSteps.length === 0 && !isStreaming) return null

  const isGOS = activeSteps.some((s) => s.id.startsWith('gos:'))
  const doneCount = activeSteps.filter((s) => s.status === 'done').length
  const hasActive = activeSteps.some((s) => s.status === 'active')
  const hasError = activeSteps.some((s) => s.status === 'error')
  const hasEvidence = activeSteps.some((s) => ['metrics', 'logs', 'knowledge', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:'))
  const evidenceTypes = isGOS
    ? ['hypothesis', 'evidence', 'confidence']
    : [
        activeSteps.some((s) => s.id === 'metrics') ? 'metrics' : '',
        activeSteps.some((s) => s.id === 'logs') ? 'logs' : '',
        activeSteps.some((s) => s.id === 'knowledge') ? 'knowledge' : '',
      ].filter(Boolean)
  const toolCount = activeSteps.filter((s) => ['metrics', 'logs', 'knowledge', 'gos:experts', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:')).length
  const errorCount = activeSteps.filter((s) => s.status === 'error').length
  const processName = isGOS ? 'GoS 信念推理' : hasEvidence ? '诊断过程' : '执行过程'
  const ProcessIcon = isGOS ? Brain : Activity
  const summary = hasActive
    ? '执行中'
    : hasError
      ? '部分失败'
      : `完成 ${doneCount} 步`
  const stepTypeLabels: Record<string, string> = {
    intent: '请求',
    context: '上下文',
    metrics: '指标',
    logs: '日志',
    knowledge: '知识库',
    reporter: '输出',
    'gos:hypothesis': '假设',
    'gos:experts': '专家',
    'gos:evidence': '证据',
    'gos:confidence': '置信度',
    'gos:reporter': '输出',
  }

  return (
    <div className="mb-3">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs transition-colors hover:bg-white/50 dark:hover:bg-slate-700/30"
      >
        <ProcessIcon size={13} className={hasError ? 'text-rose-400' : 'text-amber-500'} />
        <span className="font-medium text-zinc-600 dark:text-zinc-300">
          {processName}
        </span>
        <span className="text-zinc-400 dark:text-zinc-600">
          {summary}
        </span>
        {evidenceTypes.length > 0 && (
          <span className="hidden rounded-full bg-sky-400/15 px-2 py-0.5 text-[10px] font-semibold text-sky-600 dark:text-sky-400 sm:inline-flex">
            {evidenceTypes.join(' / ')}
          </span>
        )}
        {!isGOS && toolCount > 0 && (
          <span className="hidden text-[10px] text-zinc-400 dark:text-zinc-600 sm:inline">
            {toolCount} 类证据
          </span>
        )}
        {errorCount > 0 && (
          <span className="hidden rounded-full bg-rose-400/15 px-2 py-0.5 text-[10px] font-semibold text-rose-500 sm:inline-flex">
            {errorCount} 个异常
          </span>
        )}
        <motion.span
          animate={{ rotate: open ? 180 : 0 }}
          transition={{ duration: 0.2 }}
          className="ml-auto text-zinc-400"
        >
          <ChevronDown size={14} />
        </motion.span>
      </button>

      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden"
          >
            <div className="ml-1.5 mt-1 space-y-0.5 border-l-2 border-sky-300/40 pl-3 dark:border-sky-600/30">
              {activeSteps.map((step) => {
                const detail = stepToDetail(step)
                const isClickable = detail !== null && onStepClick

                return (
                  <div
                    key={step.id}
                    className={`py-1 text-xs ${isClickable ? 'cursor-pointer rounded-md px-1.5 -mx-1.5 transition-colors hover:bg-white/60 dark:hover:bg-slate-700/30' : ''}`}
                    onClick={isClickable ? () => onStepClick(detail!) : undefined}
                    role={isClickable ? 'button' : undefined}
                    tabIndex={isClickable ? 0 : undefined}
                    onKeyDown={isClickable ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onStepClick(detail!) } } : undefined}
                  >
                    <div className="flex min-w-0 items-center gap-2">
                      {step.status === 'active' ? (
                        <span className="h-2 w-2 shrink-0 rounded-full bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.5)] animate-pulse" />
                      ) : step.status === 'done' ? (
                        <span className="h-2 w-2 shrink-0 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
                      ) : step.status === 'error' ? (
                        <span className="h-2 w-2 shrink-0 rounded-full bg-rose-400 shadow-[0_0_6px_rgba(251,113,133,0.5)]" />
                      ) : (
                        <span className="h-2 w-2 shrink-0 rounded-full bg-zinc-300 dark:bg-zinc-700" />
                      )}
                      <span className={step.status === 'active' ? 'font-medium text-sky-600 dark:text-sky-400' : step.status === 'error' ? 'text-rose-400' : 'text-zinc-700 dark:text-zinc-300'}>
                        {step.label}
                      </span>
                      <span className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
                        step.id.startsWith('gos:')
                          ? 'bg-amber-400/15 text-amber-600 dark:text-amber-400'
                          : 'bg-white/50 text-zinc-400 dark:bg-slate-700/50 dark:text-zinc-500'
                      }`}>
                        {stepTypeLabels[step.id] || '工具'}
                      </span>
                      {step.detail && (
                        <span className="min-w-0 truncate text-zinc-400 dark:text-zinc-600">{step.detail}</span>
                      )}
                      {isClickable && (
                        <span className="ml-auto text-[9px] text-zinc-300 opacity-0 transition-opacity group-hover:opacity-100 dark:text-zinc-700">详情</span>
                      )}
                    </div>
                    {step.meta && step.meta.length > 0 && (
                      <div className="ml-5 mt-1 space-y-0.5 text-[11px] leading-5 text-zinc-400 dark:text-zinc-600">
                        {step.meta.slice(-3).map((item) => (
                          <div key={item} className={isGOS ? 'break-words' : 'truncate'}>
                            {item}
                          </div>
                        ))}
                      </div>
                    )}
                    {isGOS && step.id === 'gos:hypothesis' && step.status === 'done' && step.detail && (
                      <div className="mt-1.5 ml-5 rounded-lg border border-amber-300/30 bg-amber-50/30 px-3 py-2 dark:border-amber-600/20 dark:bg-amber-900/10">
                        <span className="text-[10px] font-semibold uppercase tracking-wider text-amber-600 dark:text-amber-400">Hypothesis</span>
                        <p className="mt-1 text-xs text-zinc-700 dark:text-zinc-300">{step.detail}</p>
                      </div>
                    )}
                    {isGOS && step.id === 'gos:experts' && step.status === 'done' && step.detail && (
                      <div className="mt-1 ml-5 flex items-center gap-1.5 text-[11px] text-zinc-500">
                        <Search size={10} className="text-sky-400" />
                        {step.detail}
                      </div>
                    )}
                    {isGOS && step.id === 'gos:evidence' && step.status === 'done' && step.meta && step.meta.length > 0 && (
                      <div className="mt-1.5 ml-5 space-y-1">
                        {step.meta.map((item, i) => (
                          <div key={i} className="flex items-start gap-2 rounded-md border border-white/40 bg-white/40 px-2.5 py-1.5 text-[11px] text-zinc-600 backdrop-blur-sm dark:border-white/5 dark:bg-slate-800/30 dark:text-zinc-400">
                            <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-sky-400/60" />
                            <span className="break-words">{item}</span>
                          </div>
                        ))}
                      </div>
                    )}
                    {isGOS && step.id === 'gos:confidence' && step.status === 'done' && (
                      <div className="mt-1 ml-5">
                        <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-medium ${
                          step.detail?.includes('降级')
                            ? 'bg-amber-400/15 text-amber-600 dark:text-amber-400'
                            : 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400'
                        }`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${
                            step.detail?.includes('降级') ? 'bg-amber-400 shadow-[0_0_6px_rgba(251,191,36,0.5)]' : 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]'
                          }`} />
                          {step.detail || 'frontier 已收敛'}
                        </span>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
