import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Activity,
  AlertTriangle,
  Brain,
  Check,
  ChevronDown,
  CircleDot,
  Clock3,
  Database,
  FileSearch,
  Gauge,
  MessageSquareText,
  Route,
  Search,
} from 'lucide-react'
import type { ChatExecutionStep } from '../../types/chat'
import type { DetailItem } from '../workbench/DetailPanel'

export type ThinkingStep = ChatExecutionStep

interface Props {
  steps: ThinkingStep[]
  isStreaming?: boolean
  defaultOpen?: boolean
  onStepClick?: (item: DetailItem) => void
}

const STEP_KIND_LABELS: Record<string, string> = {
  intent: '请求',
  context: '上下文',
  metrics: '指标',
  logs: '日志',
  knowledge: '知识库',
  reporter: '输出',
  engine: '引擎',
  dispatch: '调度',
  evidence: '证据',
  contract: '校验',
  schema: '质量',
  'gos:hypothesis': '假设',
  'gos:experts': '专家',
  'gos:evidence': '证据',
  'gos:confidence': '置信',
  'gos:reporter': '报告',
}

function stepToDetail(step: ThinkingStep): DetailItem | null {
  if (!step.detail && (!step.meta || step.meta.length === 0) && step.status !== 'error') return null
  const typeMap: Record<string, DetailItem['type']> = {
    metrics: 'metric',
    logs: 'log',
    knowledge: 'knowledge',
    evidence: 'metric',
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

function resolveStepIcon(step: ThinkingStep) {
  if (step.status === 'error') return AlertTriangle
  if (step.id === 'intent') return CircleDot
  if (step.id === 'context') return Database
  if (step.id === 'metrics') return Gauge
  if (step.id === 'logs') return Search
  if (step.id === 'knowledge') return FileSearch
  if (step.id === 'reporter' || step.id.endsWith(':reporter')) return MessageSquareText
  if (step.id.startsWith('gos:')) return Brain
  if (step.id.startsWith('tool:')) return Route
  return Activity
}

function statusClasses(status: ThinkingStep['status']): string {
  if (status === 'active') return 'border-sky-300 bg-sky-50 text-sky-700 shadow-[0_0_0_3px_rgba(14,165,233,0.08)] dark:border-sky-500/40 dark:bg-sky-500/10 dark:text-sky-300'
  if (status === 'done') return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-500/40 dark:bg-emerald-500/10 dark:text-emerald-300'
  if (status === 'error') return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-300'
  return 'border-zinc-200 bg-white text-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-500'
}

function dotClasses(status: ThinkingStep['status']): string {
  if (status === 'active') return 'bg-sky-500 shadow-[0_0_0_5px_rgba(14,165,233,0.10)]'
  if (status === 'done') return 'bg-emerald-500'
  if (status === 'error') return 'bg-amber-500'
  return 'bg-zinc-300 dark:bg-zinc-700'
}

function summarizeEvidence(steps: ThinkingStep[], isGOS: boolean): string | null {
  if (isGOS) return '假设 / 证据 / 置信'
  const types = [
    steps.some((s) => s.id === 'metrics') ? '指标' : '',
    steps.some((s) => s.id === 'logs') ? '日志' : '',
    steps.some((s) => s.id === 'knowledge') ? '知识库' : '',
    steps.some((s) => s.id.startsWith('tool:')) ? '工具' : '',
  ].filter(Boolean)
  return types.length > 0 ? types.join(' / ') : null
}

function compactText(value: string, max = 88): string {
  const text = value.replace(/\s+/g, ' ').trim()
  return text.length > max ? `${text.slice(0, max)}...` : text
}

export function ThinkingCollapse({ steps, isStreaming, defaultOpen, onStepClick }: Props) {
  const [open, setOpen] = useState(defaultOpen ?? Boolean(isStreaming))
  const visibleSteps = steps.filter((s) => isStreaming || s.status !== 'pending')

  if (visibleSteps.length === 0 && !isStreaming) return null

  const isGOS = visibleSteps.some((s) => s.id.startsWith('gos:'))
  const doneCount = visibleSteps.filter((s) => s.status === 'done').length
  const hasActive = visibleSteps.some((s) => s.status === 'active')
  const hasError = visibleSteps.some((s) => s.status === 'error')
  const evidenceSummary = summarizeEvidence(visibleSteps, isGOS)
  const processName = isGOS ? 'GoS 信念推理' : evidenceSummary ? '诊断过程' : '执行过程'
  const ProcessIcon = isGOS ? Brain : Activity
  const summary = hasActive
    ? '正在取证'
    : hasError
      ? `完成，有 ${visibleSteps.filter((s) => s.status === 'error').length} 个降级点`
      : `已完成 ${doneCount} 步`

  return (
    <div className="mb-3">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="group flex w-full items-center gap-2 rounded-lg border border-zinc-200/70 bg-zinc-50/70 px-3 py-2 text-left text-xs shadow-[0_1px_0_rgba(15,23,42,0.03)] transition-colors hover:border-sky-200 hover:bg-white/85 dark:border-zinc-800/70 dark:bg-zinc-900/45 dark:hover:border-sky-800/70 dark:hover:bg-zinc-900/80"
      >
        <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-md border ${hasError ? 'border-amber-300 bg-amber-50 text-amber-600 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-300' : hasActive ? 'border-sky-300 bg-sky-50 text-sky-600 dark:border-sky-500/40 dark:bg-sky-500/10 dark:text-sky-300' : 'border-emerald-300 bg-emerald-50 text-emerald-600 dark:border-emerald-500/40 dark:bg-emerald-500/10 dark:text-emerald-300'}`}>
          <ProcessIcon size={13} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-2">
            <span className="shrink-0 font-semibold text-zinc-800 dark:text-zinc-100">{processName}</span>
            <span className="truncate text-zinc-500 dark:text-zinc-500">{summary}</span>
          </span>
          {evidenceSummary && (
            <span className="mt-0.5 block truncate text-[11px] text-zinc-400 dark:text-zinc-600">
              证据链：{evidenceSummary}
            </span>
          )}
        </span>
        <span className="hidden items-center gap-1.5 sm:flex">
          {visibleSteps.slice(0, 6).map((step) => (
            <span key={step.id} className={`h-1.5 w-1.5 rounded-full ${dotClasses(step.status)}`} />
          ))}
        </span>
        <motion.span
          animate={{ rotate: open ? 180 : 0 }}
          transition={{ duration: 0.18 }}
          className="shrink-0 text-zinc-400 transition-colors group-hover:text-zinc-600 dark:group-hover:text-zinc-300"
        >
          <ChevronDown size={14} />
        </motion.span>
      </button>

      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0, y: -4 }}
            animate={{ height: 'auto', opacity: 1, y: 0 }}
            exit={{ height: 0, opacity: 0, y: -4 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden"
          >
            <div className="mt-2 rounded-lg border border-zinc-200/60 bg-white/50 p-2 dark:border-zinc-800/60 dark:bg-zinc-950/20">
              <div className="space-y-1">
                {visibleSteps.map((step, index) => {
                  const StepIcon = resolveStepIcon(step)
                  const detail = stepToDetail(step)
                  const isClickable = detail !== null && onStepClick
                  const isLast = index === visibleSteps.length - 1

                  return (
                    <div
                      key={step.id}
                      className={`group/item relative grid grid-cols-[22px_1fr] gap-2 rounded-md px-1.5 py-1.5 ${isClickable ? 'cursor-pointer hover:bg-zinc-50 dark:hover:bg-zinc-900/60' : ''}`}
                      onClick={isClickable ? () => onStepClick(detail!) : undefined}
                      role={isClickable ? 'button' : undefined}
                      tabIndex={isClickable ? 0 : undefined}
                      onKeyDown={isClickable ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onStepClick(detail!) } } : undefined}
                    >
                      {!isLast && (
                        <span className="absolute left-[12px] top-7 h-[calc(100%-18px)] w-px bg-zinc-200 dark:bg-zinc-800" />
                      )}
                      <span className={`relative z-10 flex h-5 w-5 items-center justify-center rounded-md border ${statusClasses(step.status)} ${step.status === 'active' ? 'animate-pulse' : ''}`}>
                        {step.status === 'done' ? <Check size={11} /> : step.status === 'pending' ? <Clock3 size={11} /> : <StepIcon size={11} />}
                      </span>
                      <span className="min-w-0">
                        <span className="flex min-w-0 items-center gap-2">
                          <span className={`truncate text-[12px] font-medium ${step.status === 'error' ? 'text-amber-700 dark:text-amber-300' : step.status === 'active' ? 'text-sky-700 dark:text-sky-300' : 'text-zinc-700 dark:text-zinc-300'}`}>
                            {step.label}
                          </span>
                          <span className="shrink-0 rounded-full bg-zinc-100 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 dark:bg-zinc-800 dark:text-zinc-500">
                            {STEP_KIND_LABELS[step.id] || (step.id.startsWith('tool:') ? '工具' : '阶段')}
                          </span>
                          {isClickable && (
                            <span className="ml-auto hidden text-[10px] text-zinc-400 opacity-0 transition-opacity group-hover/item:opacity-100 sm:inline">
                              查看
                            </span>
                          )}
                        </span>
                        {step.detail && (
                          <span className="mt-0.5 block truncate text-[11px] leading-5 text-zinc-500 dark:text-zinc-500">
                            {compactText(step.detail)}
                          </span>
                        )}
                        {step.meta && step.meta.length > 0 && (
                          <span className="mt-1 block space-y-1">
                            {step.meta.slice(-3).map((item) => (
                              <span
                                key={item}
                                className="block truncate rounded-md bg-zinc-50 px-2 py-1 text-[11px] leading-4 text-zinc-500 ring-1 ring-inset ring-zinc-100 dark:bg-zinc-900/60 dark:text-zinc-500 dark:ring-zinc-800"
                              >
                                {compactText(item, isGOS ? 120 : 96)}
                              </span>
                            ))}
                          </span>
                        )}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
