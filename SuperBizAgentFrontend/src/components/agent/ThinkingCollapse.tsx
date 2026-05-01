import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { ChevronDown, Brain, Loader2, Check } from 'lucide-react'

export interface ThinkingStep {
  id: string
  label: string
  status: 'pending' | 'active' | 'done' | 'error'
  detail?: string
}

interface Props {
  steps: ThinkingStep[]
  isStreaming?: boolean
}

export function ThinkingCollapse({ steps, isStreaming }: Props) {
  const [open, setOpen] = useState(false)
  const activeSteps = steps.filter((s) => s.status !== 'pending')

  if (activeSteps.length === 0 && !isStreaming) return null

  const doneCount = activeSteps.filter((s) => s.status === 'done').length
  const hasActive = activeSteps.some((s) => s.status === 'active')

  return (
    <div className="mb-3">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-800/50"
      >
        <Brain size={13} className="text-accent" />
        <span className="text-zinc-500 dark:text-zinc-400">
          {hasActive ? '正在思考...' : isStreaming ? '思考完成' : `已深度思考 (${doneCount} 步)`}
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
            <div className="mt-1 space-y-0.5 border-l-2 border-accent/20 ml-1.5 pl-3">
              {activeSteps.map((step) => (
                <div key={step.id} className="flex items-center gap-2 py-0.5 text-xs">
                  {step.status === 'active' ? (
                    <Loader2 size={11} className="shrink-0 animate-spin text-accent" />
                  ) : step.status === 'done' ? (
                    <Check size={11} className="shrink-0 text-emerald-400" />
                  ) : (
                    <span className="shrink-0 text-[10px] text-red-400">!</span>
                  )}
                  <span className={step.status === 'pending' ? 'text-zinc-400' : step.status === 'active' ? 'text-accent font-medium' : 'text-zinc-600 dark:text-zinc-400'}>
                    {step.label}
                  </span>
                  {step.detail && (
                    <span className="truncate text-zinc-400 dark:text-zinc-600">{step.detail}</span>
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
