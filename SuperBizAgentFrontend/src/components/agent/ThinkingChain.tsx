import { motion, AnimatePresence } from 'framer-motion'
import { Check, Loader2, Brain, Search, FileText, BarChart3, BookOpen, MessageSquare } from 'lucide-react'

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

const stepIcons: Record<string, typeof Brain> = {
  triage: Search,
  metrics: BarChart3,
  logs: FileText,
  knowledge: BookOpen,
  reporter: MessageSquare,
  default: Brain,
}

const stepColors: Record<string, string> = {
  pending: 'text-zinc-300 dark:text-zinc-700',
  active: 'text-accent',
  done: 'text-emerald-400',
  error: 'text-red-400',
}

const stepBgs: Record<string, string> = {
  pending: 'bg-zinc-100 dark:bg-zinc-800',
  active: 'bg-accent/10',
  done: 'bg-emerald-500/10',
  error: 'bg-red-500/10',
}

export function ThinkingChain({ steps, isStreaming }: Props) {
  if (steps.length === 0 && !isStreaming) return null

  const doneCount = steps.filter(s => s.status === 'done').length
  const hasActivity = steps.some(s => s.status !== 'pending') || isStreaming
  if (!hasActivity && !isStreaming) return null

  return (
    <div className="rounded-2xl border border-zinc-200/80 bg-white/80 px-4 py-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <div className="mb-2 flex items-center gap-2">
        <Brain size={14} className="text-accent" />
        <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">
          {isStreaming ? '执行中...' : doneCount > 0 ? `执行完成 ${doneCount}/${steps.length} 步` : '准备执行...'}
        </span>
      </div>

      <div className="space-y-1">
        <AnimatePresence>
          {steps.map((step, i) => {
            const Icon = stepIcons[step.id] || stepIcons.default
            return (
              <motion.div
                key={step.id}
                initial={{ opacity: 0, x: -8 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: -8 }}
                transition={{ duration: 0.2, delay: i * 0.05 }}
                className={`flex items-center gap-2.5 rounded-lg px-3 py-2 text-xs ${stepBgs[step.status]}`}
              >
                {/* Icon */}
                <div className={`shrink-0 ${stepColors[step.status]}`}>
                  {step.status === 'active' ? (
                    <Loader2 size={13} className="animate-spin" />
                  ) : step.status === 'done' ? (
                    <Check size={13} />
                  ) : step.status === 'error' ? (
                    <span className="text-[10px] font-bold">!</span>
                  ) : (
                    <Icon size={13} />
                  )}
                </div>

                {/* Label */}
                <span className={`font-medium ${stepColors[step.status]}`}>
                  {step.label}
                </span>

                {/* Detail */}
                {step.detail && (
                  <span className="truncate text-zinc-400 dark:text-zinc-600">
                    {step.detail}
                  </span>
                )}

                {/* Animated bar for active */}
                {step.status === 'active' && (
                  <motion.div
                    className="ml-auto h-0.5 w-8 rounded-full bg-accent/30"
                    animate={{ opacity: [0.3, 1, 0.3] }}
                    transition={{ duration: 1.5, repeat: Infinity }}
                  />
                )}
              </motion.div>
            )
          })}
        </AnimatePresence>
      </div>
    </div>
  )
}
