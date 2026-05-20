import { useState } from 'react'
import { motion } from 'framer-motion'
import { ArrowRight, Check } from 'lucide-react'
import type { ThinkingStep } from '../agent/ThinkingCollapse'

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

export function ResultCard({ steps, degraded, degradationReason, onRefine, onExport }: Props) {
  const risk = assessRisk(steps)
  const evidence = countEvidence(steps)
  const isGoS = steps.some((s) => s.id.startsWith('gos:'))
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

      <div className="relative rounded-[22px] rounded-bl-[6px] border border-white/60 bg-white/70 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-800/60">
        <div className="flex items-center justify-between border-b border-white/40 px-4 py-3 dark:border-white/5">
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
              {isGoS ? 'GoS 诊断结果' : '诊断结果'}
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
            className="flex items-center gap-1.5 rounded-full border border-white/40 bg-white/50 px-3.5 py-1.5 text-[11px] font-semibold text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:bg-white hover:text-sky-600 hover:shadow-md dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-400 dark:hover:bg-slate-600 dark:hover:text-sky-400"
          >
            <ArrowRight size={12} />
            继续排查
          </button>
          <button
            type="button"
            onClick={handleExport}
            className="flex items-center gap-1.5 rounded-full border border-white/40 bg-white/50 px-3.5 py-1.5 text-[11px] font-semibold text-zinc-600 backdrop-blur-sm transition-all hover:-translate-y-0.5 hover:bg-white hover:text-sky-600 hover:shadow-md dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-400 dark:hover:bg-slate-600 dark:hover:text-sky-400"
          >
            {exported ? <Check size={12} className="text-emerald-500" /> : <ArrowRight size={12} />}
            {exported ? '已复制' : '导出报告'}
          </button>
        </div>
      </div>
    </motion.div>
  )
}
