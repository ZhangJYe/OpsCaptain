import { motion } from 'framer-motion'
import { Brain, Loader2, CheckCircle2, CircleAlert, Route, FlaskConical, FileSearch, BarChart3 } from 'lucide-react'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { ENGINE_VIEW_MODEL } from '../../lib/engineViewModel'

interface Props {
  steps: ThinkingStep[]
  content?: string
}

const BELIEF_NODES: Array<{
  id: string
  label: string
  description: string
  icon: typeof Brain
}> = [
  { id: 'gos:hypothesis', label: 'Hypothesis', description: '建立候选假设', icon: FlaskConical },
  { id: 'gos:experts', label: 'Experts', description: '调度专家检索', icon: Route },
  { id: 'gos:evidence', label: 'Evidence', description: '挂载支持证据', icon: FileSearch },
  { id: 'gos:confidence', label: 'Confidence', description: '校准置信度', icon: BarChart3 },
  { id: 'gos:reporter', label: 'Report', description: '生成证据链报告', icon: Brain },
]

export function GoSBeliefProgress({ steps, content }: Props) {
  const view = ENGINE_VIEW_MODEL.gos_engine
  const activeNodes = BELIEF_NODES.filter((node) => {
    const step = steps.find((s) => s.id === node.id)
    return step && step.status !== 'pending'
  })

  return (
    <div className="ml-10 overflow-hidden rounded-[22px] rounded-bl-[6px] border border-amber-200/60 bg-white/65 backdrop-blur-xl dark:border-amber-500/15 dark:bg-slate-800/45">
      <div className="flex items-center justify-between border-b border-white/40 px-4 py-3 dark:border-white/5">
        <div className="flex items-center gap-2">
          <span className={`flex h-7 w-7 items-center justify-center rounded-lg ring-1 ${view.sidebar.icon}`}>
            <Brain size={14} />
          </span>
          <div>
            <p className="text-xs font-semibold text-amber-700 dark:text-amber-300">Belief Graph</p>
            <p className="text-[10px] text-zinc-400 dark:text-zinc-600">{view.trace}</p>
          </div>
        </div>
        <span className={`rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ${view.sidebar.flowActive}`}>
          {activeNodes.length}/{BELIEF_NODES.length}
        </span>
      </div>

      <div className="px-4 py-3">
        <div className="relative">
          {activeNodes.length < BELIEF_NODES.length && activeNodes.length > 0 && (
            <motion.div
              className="absolute -left-1 top-0 h-full w-0.5 rounded-full bg-amber-300/30 dark:bg-amber-500/20"
              initial={{ scaleY: 0 }}
              animate={{ scaleY: 1 }}
              style={{ originY: 0 }}
            />
          )}

          <div className="space-y-2">
            {BELIEF_NODES.map((node) => {
              const step = steps.find((s) => s.id === node.id)
              const status = step?.status || 'pending'
              if (status === 'pending') return null

              const Icon = node.icon
              const isLast = node.id === BELIEF_NODES[BELIEF_NODES.length - 1].id

              return (
                <div key={node.id} className="relative">
                  {!isLast && status === 'done' && (
                    <span className="absolute left-[9px] top-5 h-[calc(100%-6px)] w-px bg-amber-200/50 dark:bg-amber-500/15" />
                  )}

                  <div className="grid grid-cols-[20px_minmax(0,1fr)] gap-3">
                    <span className="relative flex justify-center">
                      <span className={`mt-0.5 flex h-5 w-5 items-center justify-center rounded-full ring-1 ${
                        status === 'active'
                          ? 'bg-amber-500/15 ring-amber-400/30'
                          : status === 'done'
                          ? 'bg-emerald-500/10 ring-emerald-400/20'
                          : status === 'error'
                          ? 'bg-rose-500/10 ring-rose-400/20'
                          : 'bg-white ring-amber-200 dark:bg-slate-800 dark:ring-amber-500/20'
                      }`}>
                        {status === 'active' ? (
                          <Loader2 size={12} className="animate-spin text-amber-500" />
                        ) : status === 'done' ? (
                          <CheckCircle2 size={13} className="text-emerald-500" />
                        ) : status === 'error' ? (
                          <CircleAlert size={13} className="text-rose-500" />
                        ) : (
                          <Icon size={10} className="text-amber-400/60" />
                        )}
                      </span>
                    </span>

                    <div className="min-w-0 pb-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className={`text-xs font-semibold ${
                          status === 'active' ? 'text-amber-700 dark:text-amber-300' : 'text-zinc-700 dark:text-zinc-300'
                        }`}>
                          {step?.label || node.label}
                        </span>
                        <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${view.sidebar.flowActive}`}>
                          {node.description}
                        </span>
                        {status === 'active' && (
                          <span className="text-[10px] font-medium text-amber-500">推理中...</span>
                        )}
                      </div>

                      {step?.detail && (
                        <p className="mt-1 truncate text-[11px] text-zinc-500 dark:text-zinc-500">
                          {step.detail}
                        </p>
                      )}

                      {step?.meta && step.meta.length > 0 && (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {step.meta.slice(-3).map((item) => (
                            <span
                              key={item}
                              className="max-w-[220px] truncate rounded-md bg-amber-50/50 px-2 py-0.5 text-[10px] text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
                            >
                              {item}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>

        {content && (
          <>
            <div className="my-2.5 border-t border-amber-200/30 dark:border-amber-500/10" />
            <div className="prose-chat text-xs leading-6 text-zinc-600 dark:text-zinc-400">
              {content}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
