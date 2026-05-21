import { useState } from 'react'
import { motion } from 'framer-motion'
import { ArrowRight, Brain, Check, Route } from 'lucide-react'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { ENGINE_VIEW_MODEL } from '../../lib/engineViewModel'

interface Props {
  steps: ThinkingStep[]
  degraded?: boolean
  degradationReason?: string
  onRefine?: () => void
  onExport?: () => Promise<void>
}

function assessRisk(steps: ThinkingStep[]): { level: 'low' | 'medium' | 'high'; label: string; dotColor: string; dotShadow: string } {
  const hasError = steps.some((s) => s.status === 'error')
  const errorCount = steps.filter((s) => s.status === 'error').length
  const evidenceCount = steps.filter((s) => ['metrics', 'logs', 'knowledge', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:')).length

  if (hasError && errorCount >= 2) {
    return { level: 'high', label: '高风险', dotColor: 'bg-rose-400', dotShadow: 'shadow-[0_0_8px_rgba(251,113,133,0.5)]' }
  }
  if (hasError || evidenceCount < 2) {
    return { level: 'medium', label: '中风险', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]' }
  }
  return { level: 'low', label: '低风险', dotColor: 'bg-emerald-400', dotShadow: 'shadow-[0_0_8px_rgba(52,211,153,0.5)]' }
}

function countEvidence(steps: ThinkingStep[]): { metrics: boolean; logs: boolean; knowledge: boolean; total: number } {
  return {
    metrics: steps.some((s) => s.id === 'metrics' && s.status === 'done'),
    logs: steps.some((s) => s.id === 'logs' && s.status === 'done'),
    knowledge: steps.some((s) => s.id === 'knowledge' && s.status === 'done'),
    total: steps.filter((s) => (['metrics', 'logs', 'knowledge', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:')) && s.status === 'done').length,
  }
}

function getGoSSummary(steps: ThinkingStep[]) {
  const hypothesis = steps.find((s) => s.id === 'gos:hypothesis')
  const evidence = steps.find((s) => s.id === 'gos:evidence')
  const confidence = steps.find((s) => s.id === 'gos:confidence')
  const evidenceItems = [evidence?.detail, ...(evidence?.meta || [])].filter(Boolean)

  return {
    hypothesis: hypothesis?.detail || '候选根因已建立',
    evidence: evidenceItems.length > 0 ? `${evidenceItems.length} 条支持证据` : '等待证据挂载',
    confidence: confidence?.detail || '置信度校准中',
  }
}

export function ResultCard({ steps, degraded, degradationReason, onRefine, onExport }: Props) {
  const risk = assessRisk(steps)
  const evidence = countEvidence(steps)
  const isGoS = steps.some((s) => s.id.startsWith('gos:'))
  const view = isGoS ? ENGINE_VIEW_MODEL.gos_engine : ENGINE_VIEW_MODEL.plan_execute_replan
  const gosSummary = getGoSSummary(steps)
  const HeaderIcon = isGoS ? Brain : Route
  const [exported, setExported] = useState(false)

  const handleExport = async () => {
    if (!onExport) return
    try {
      await onExport()
      setExported(true)
      setTimeout(() => setExported(false), 2000)
    } catch {
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', damping: 20, stiffness: 200 }}
      className="relative"
    >
      <div aria-hidden="true" className="glow-frame rounded-[22px] rounded-bl-[6px]" />

      <div className={`relative rounded-[22px] rounded-bl-[6px] border border-white/60 bg-white/70 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-800/60 ${isGoS ? 'border-l-2 border-l-amber-400' : 'border-l-2 border-l-sky-400'}`}>
        <div className="flex items-center justify-between border-b border-white/40 px-4 py-3 dark:border-white/5">
          <div className="flex items-center gap-2">
            <span className={`flex h-7 w-7 items-center justify-center rounded-lg ring-1 ${view.sidebar.icon}`}>
              <HeaderIcon size={14} />
            </span>
            <span>
              <span className="block text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                {view.resultTitle}
              </span>
              <span className="mt-0.5 hidden text-[10px] leading-4 text-zinc-400 dark:text-zinc-600 sm:block">
                {view.resultSubtitle}
              </span>
            </span>
            {degraded && (
              <span className="rounded-full bg-amber-400/15 px-2 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
                已降级
              </span>
            )}
          </div>
          <div className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${risk.dotColor} ${risk.dotShadow} animate-pulse`} />
            <span className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-400">{risk.label}</span>
          </div>
        </div>

        {isGoS ? (
          <div className="grid gap-2 px-4 py-3 sm:grid-cols-3">
            {[
              { label: '主假设', value: gosSummary.hypothesis },
              { label: '支持证据', value: gosSummary.evidence },
              { label: '置信度', value: gosSummary.confidence },
            ].map((item) => (
              <div key={item.label} className="rounded-xl border border-amber-200/50 bg-amber-50/35 px-3 py-2 dark:border-amber-500/15 dark:bg-amber-500/10">
                <p className="text-[10px] font-semibold text-amber-600 dark:text-amber-300">{item.label}</p>
                <p className="mt-1 line-clamp-2 text-[11px] leading-5 text-zinc-600 dark:text-zinc-300">{item.value}</p>
              </div>
            ))}
          </div>
        ) : (
          <div className="flex items-center gap-4 px-4 py-3">
            <div className="flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${evidence.metrics ? 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.5)]' : 'bg-zinc-300 dark:bg-zinc-700'}`} />
              <span className={`text-[11px] ${evidence.metrics ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>指标</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${evidence.logs ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]' : 'bg-zinc-300 dark:bg-zinc-700'}`} />
              <span className={`text-[11px] ${evidence.logs ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>日志</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${evidence.knowledge ? 'bg-violet-400 shadow-[0_0_6px_rgba(167,139,250,0.5)]' : 'bg-zinc-300 dark:bg-zinc-700'}`} />
              <span className={`text-[11px] ${evidence.knowledge ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>知识库</span>
            </div>
            <div className="ml-auto text-[11px] text-zinc-400 dark:text-zinc-600">
              {evidence.total} 条证据
            </div>
          </div>
        )}

        {degraded && degradationReason && (
          <div className="border-t border-white/40 px-4 py-2 dark:border-white/5">
            <p className="text-[11px] text-amber-600 dark:text-amber-400">
              降级原因：{degradationReason}
            </p>
          </div>
        )}

        <div className="flex items-center gap-2 border-t border-white/40 px-4 py-3 dark:border-white/5">
          <button
            type="button"
            onClick={onRefine}
            className={`flex items-center gap-1.5 rounded-full border border-white/40 bg-white/50 px-3.5 py-1.5 text-[11px] font-semibold text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:bg-white hover:shadow-md dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-400 dark:hover:bg-slate-600 ${view.actionButton}`}
          >
            <ArrowRight size={12} />
            {view.resultPrimaryAction}
          </button>
          <button
            type="button"
            onClick={handleExport}
            className={`flex items-center gap-1.5 rounded-full border border-white/40 bg-white/50 px-3.5 py-1.5 text-[11px] font-semibold text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:bg-white hover:shadow-md dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-400 dark:hover:bg-slate-600 ${view.actionButton}`}
          >
            {exported ? <Check size={12} className="text-emerald-500" /> : <ArrowRight size={12} />}
            {exported ? '已复制' : '导出报告'}
          </button>
        </div>
      </div>
    </motion.div>
  )
}
