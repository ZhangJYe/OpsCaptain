import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, ChevronDown, Loader2, Check, AlertCircle } from 'lucide-react'
import type { ChatExecutionStep } from '../../types/chat'

export type ThinkingStep = ChatExecutionStep

interface Props {
  steps: ThinkingStep[]
  isStreaming?: boolean
  defaultOpen?: boolean
}

export function ThinkingCollapse({ steps, isStreaming, defaultOpen }: Props) {
  const [open, setOpen] = useState(defaultOpen ?? Boolean(isStreaming))
  const activeSteps = steps.filter((s) => s.status !== 'pending')

  if (activeSteps.length === 0 && !isStreaming) return null

  const doneCount = activeSteps.filter((s) => s.status === 'done').length
  const hasActive = activeSteps.some((s) => s.status === 'active')
  const hasError = activeSteps.some((s) => s.status === 'error')
  const hasEvidence = activeSteps.some((s) => ['metrics', 'logs', 'knowledge'].includes(s.id) || s.id.startsWith('tool:'))
  const processName = hasEvidence ? '诊断过程' : '执行过程'
  const summary = hasActive
    ? '执行中'
    : hasError
      ? '部分失败'
      : `完成 ${doneCount} 步`

  return (
    <div className="mb-3">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-800/50"
      >
        <Activity size={13} className={hasError ? 'text-red-400' : 'text-accent'} />
        <span className="font-medium text-zinc-600 dark:text-zinc-300">
          {processName}
        </span>
        <span className="text-zinc-400 dark:text-zinc-600">
          {summary}
        </span>
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
            <div className="ml-1.5 mt-1 space-y-1 border-l-2 border-accent/20 pl-3">
              {activeSteps.map((step) => (
                <div key={step.id} className="flex min-w-0 items-center gap-2 py-0.5 text-xs">
                  {step.status === 'active' ? (
                    <Loader2 size={11} className="shrink-0 animate-spin text-accent" />
                  ) : step.status === 'done' ? (
                    <Check size={11} className="shrink-0 text-emerald-400" />
                  ) : (
                    <AlertCircle size={11} className="shrink-0 text-red-400" />
                  )}
                  <span className={step.status === 'active' ? 'font-medium text-accent' : step.status === 'error' ? 'text-red-400' : 'text-zinc-600 dark:text-zinc-400'}>
                    {step.label}
                  </span>
                  {step.detail && (
                    <span className="min-w-0 truncate text-zinc-400 dark:text-zinc-600">{step.detail}</span>
                  )}
                </div>
              ))}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
