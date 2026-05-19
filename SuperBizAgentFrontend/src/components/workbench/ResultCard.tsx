import { motion } from 'framer-motion'
import { Shield, AlertTriangle, CheckCircle, BarChart3, ScrollText, BookOpen, ArrowRight } from 'lucide-react'
import type { ThinkingStep } from '../agent/ThinkingCollapse'

interface Props {
  steps: ThinkingStep[]
  degraded?: boolean
  degradationReason?: string
  onRefine?: () => void
}

function assessRisk(steps: ThinkingStep[]): { level: 'low' | 'medium' | 'high'; label: string; color: string; bgColor: string; icon: typeof Shield } {
  const hasError = steps.some((s) => s.status === 'error')
  const errorCount = steps.filter((s) => s.status === 'error').length
  const evidenceCount = steps.filter((s) => ['metrics', 'logs', 'knowledge', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:')).length

  if (hasError && errorCount >= 2) {
    return { level: 'high', label: '高风险', color: 'text-red-500', bgColor: 'bg-red-500/10', icon: AlertTriangle }
  }
  if (hasError || evidenceCount < 2) {
    return { level: 'medium', label: '中风险', color: 'text-amber-500', bgColor: 'bg-amber-500/10', icon: AlertTriangle }
  }
  return { level: 'low', label: '低风险', color: 'text-emerald-500', bgColor: 'bg-emerald-500/10', icon: CheckCircle }
}

function countEvidence(steps: ThinkingStep[]): { metrics: boolean; logs: boolean; knowledge: boolean; total: number } {
  return {
    metrics: steps.some((s) => s.id === 'metrics' && s.status === 'done'),
    logs: steps.some((s) => s.id === 'logs' && s.status === 'done'),
    knowledge: steps.some((s) => s.id === 'knowledge' && s.status === 'done'),
    total: steps.filter((s) => (['metrics', 'logs', 'knowledge', 'gos:evidence'].includes(s.id) || s.id.startsWith('tool:')) && s.status === 'done').length,
  }
}

export function ResultCard({ steps, degraded, degradationReason }: Props) {
  const risk = assessRisk(steps)
  const evidence = countEvidence(steps)
  const RiskIcon = risk.icon
  const isGoS = steps.some((s) => s.id.startsWith('gos:'))

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', damping: 20, stiffness: 200 }}
      className="rounded-xl border border-zinc-200/80 bg-gradient-to-br from-white to-zinc-50/80 shadow-sm dark:border-zinc-800/60 dark:from-zinc-900 dark:to-zinc-900/80"
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-100 px-4 py-3 dark:border-zinc-800/40">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
            {isGoS ? 'GoS 诊断结果' : '诊断结果'}
          </span>
          {degraded && (
            <span className="rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-500">
              已降级
            </span>
          )}
        </div>
        <div className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 ${risk.bgColor}`}>
          <RiskIcon size={12} className={risk.color} />
          <span className={`text-[11px] font-medium ${risk.color}`}>{risk.label}</span>
        </div>
      </div>

      {/* Stats */}
      <div className="flex items-center gap-4 px-4 py-3">
        <div className="flex items-center gap-1.5">
          <BarChart3 size={12} className={evidence.metrics ? 'text-blue-500' : 'text-zinc-300 dark:text-zinc-700'} />
          <span className={`text-[11px] ${evidence.metrics ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>指标</span>
        </div>
        <div className="flex items-center gap-1.5">
          <ScrollText size={12} className={evidence.logs ? 'text-emerald-500' : 'text-zinc-300 dark:text-zinc-700'} />
          <span className={`text-[11px] ${evidence.logs ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>日志</span>
        </div>
        <div className="flex items-center gap-1.5">
          <BookOpen size={12} className={evidence.knowledge ? 'text-violet-500' : 'text-zinc-300 dark:text-zinc-700'} />
          <span className={`text-[11px] ${evidence.knowledge ? 'text-zinc-600 dark:text-zinc-400' : 'text-zinc-300 dark:text-zinc-700'}`}>知识库</span>
        </div>
        <div className="ml-auto text-[11px] text-zinc-400 dark:text-zinc-600">
          {evidence.total} 条证据
        </div>
      </div>

      {/* Degradation reason */}
      {degraded && degradationReason && (
        <div className="border-t border-zinc-100 px-4 py-2 dark:border-zinc-800/40">
          <p className="text-[11px] text-amber-600 dark:text-amber-400">
            降级原因：{degradationReason}
          </p>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 border-t border-zinc-100 px-4 py-3 dark:border-zinc-800/40">
        <button
          type="button"
          className="flex items-center gap-1.5 rounded-lg border border-zinc-200/80 bg-white px-3 py-1.5 text-[11px] font-medium text-zinc-600 transition-colors hover:border-accent/30 hover:text-accent dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
        >
          <ArrowRight size={12} />
          继续排查
        </button>
        <button
          type="button"
          className="flex items-center gap-1.5 rounded-lg border border-zinc-200/80 bg-white px-3 py-1.5 text-[11px] font-medium text-zinc-600 transition-colors hover:border-accent/30 hover:text-accent dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
        >
          <ArrowRight size={12} />
          导出报告
        </button>
      </div>
    </motion.div>
  )
}
