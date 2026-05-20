import { motion } from 'framer-motion'
import {
  Loader2, Check, AlertCircle, Search, BarChart3, ScrollText,
  BookOpen, Brain, Zap, FileText, Shield, Layers,
} from 'lucide-react'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import type { DetailItem } from './DetailPanel'

interface BlockStyle {
  icon: typeof BarChart3
  label: string
  color: string
  dotColor: string
  dotShadow: string
  clickable: boolean
}

const BLOCK_STYLES: Record<string, BlockStyle> = {
  intent: { icon: Zap, label: '意图识别', color: 'text-zinc-400', dotColor: 'bg-zinc-400', dotShadow: '', clickable: false },
  context: { icon: Layers, label: '上下文装配', color: 'text-zinc-400', dotColor: 'bg-zinc-400', dotShadow: '', clickable: false },
  metrics: { icon: BarChart3, label: '指标证据', color: 'text-sky-500', dotColor: 'bg-sky-400', dotShadow: 'shadow-[0_0_8px_rgba(56,189,248,0.5)]', clickable: true },
  logs: { icon: ScrollText, label: '日志证据', color: 'text-emerald-500', dotColor: 'bg-emerald-400', dotShadow: 'shadow-[0_0_8px_rgba(52,211,153,0.5)]', clickable: true },
  knowledge: { icon: BookOpen, label: '知识库', color: 'text-violet-500', dotColor: 'bg-violet-400', dotShadow: 'shadow-[0_0_8px_rgba(167,139,250,0.5)]', clickable: true },
  reporter: { icon: FileText, label: '生成报告', color: 'text-amber-500', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]', clickable: false },
  contract: { icon: Shield, label: 'Contract 校验', color: 'text-amber-500', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]', clickable: false },
  schema: { icon: Shield, label: 'Schema 校验', color: 'text-amber-500', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]', clickable: false },
  'gos:hypothesis': { icon: Brain, label: '假设', color: 'text-amber-500', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]', clickable: false },
  'gos:experts': { icon: Search, label: '专家调度', color: 'text-sky-500', dotColor: 'bg-sky-400', dotShadow: 'shadow-[0_0_8px_rgba(56,189,248,0.5)]', clickable: false },
  'gos:evidence': { icon: BarChart3, label: '证据挂载', color: 'text-sky-500', dotColor: 'bg-sky-400', dotShadow: 'shadow-[0_0_8px_rgba(56,189,248,0.5)]', clickable: true },
  'gos:confidence': { icon: Shield, label: '置信度', color: 'text-emerald-500', dotColor: 'bg-emerald-400', dotShadow: 'shadow-[0_0_8px_rgba(52,211,153,0.5)]', clickable: false },
  'gos:reporter': { icon: FileText, label: 'GoS 报告', color: 'text-amber-500', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]', clickable: false },
}

function getBlockStyle(stepId: string): BlockStyle {
  if (BLOCK_STYLES[stepId]) return BLOCK_STYLES[stepId]
  if (stepId.startsWith('tool:')) {
    return { icon: Zap, label: '工具调用', color: 'text-indigo-500', dotColor: 'bg-indigo-400', dotShadow: 'shadow-[0_0_8px_rgba(129,140,248,0.5)]', clickable: true }
  }
  return { icon: FileText, label: '执行', color: 'text-zinc-400', dotColor: 'bg-zinc-400', dotShadow: '', clickable: false }
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

interface Props {
  step: ThinkingStep
  onOpenDetail?: (item: DetailItem) => void
}

export function EvidenceBlock({ step, onOpenDetail }: Props) {
  const style = getBlockStyle(step.id)
  const Icon = style.icon
  const detail = stepToDetail(step)
  const isCompact = detail === null

  const handleClick = () => {
    if (!isCompact && detail && onOpenDetail) {
      onOpenDetail(detail)
    }
  }

  if (isCompact) {
    return (
      <motion.div
        initial={{ opacity: 0, x: -8 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ type: 'spring', damping: 20, stiffness: 300 }}
        className="flex items-center gap-2 py-1"
      >
        {step.status === 'active' ? (
          <span className={`h-2 w-2 shrink-0 rounded-full bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.5)] animate-pulse`} />
        ) : step.status === 'done' ? (
          <span className="h-2 w-2 shrink-0 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]" />
        ) : step.status === 'error' ? (
          <span className="h-2 w-2 shrink-0 rounded-full bg-rose-400 shadow-[0_0_8px_rgba(251,113,133,0.5)]" />
        ) : (
          <span className={`h-2 w-2 shrink-0 rounded-full ${style.dotColor}`} />
        )}
        <span className="text-[11px] text-zinc-500 dark:text-zinc-500">{step.label}</span>
        {step.detail && (
          <span className="text-[11px] text-zinc-400 dark:text-zinc-600 truncate">{step.detail}</span>
        )}
      </motion.div>
    )
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', damping: 20, stiffness: 300 }}
      className={`group flex items-start gap-3 rounded-xl border border-white/60 bg-white/70 px-3 py-2.5 backdrop-blur-md dark:border-white/10 dark:bg-slate-800/40 ${
        !isCompact ? 'cursor-pointer transition-all hover:-translate-y-0.5 hover:bg-white/90 hover:shadow-md dark:hover:bg-slate-800/60' : ''
      }`}
      onClick={handleClick}
      role={!isCompact ? 'button' : undefined}
      tabIndex={!isCompact ? 0 : undefined}
      onKeyDown={!isCompact ? (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleClick() }
      } : undefined}
    >
      <div className="mt-1 shrink-0">
        {step.status === 'active' ? (
          <span className={`block h-2 w-2 rounded-full bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.5)] animate-pulse`} />
        ) : step.status === 'done' ? (
          <span className="block h-2 w-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]" />
        ) : step.status === 'error' ? (
          <span className="block h-2 w-2 rounded-full bg-rose-400 shadow-[0_0_8px_rgba(251,113,133,0.5)]" />
        ) : (
          <span className={`block h-2 w-2 rounded-full ${style.dotColor}`} />
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className={`text-xs font-medium ${step.status === 'active' ? 'text-sky-600 dark:text-sky-400' : step.status === 'error' ? 'text-rose-500' : 'text-zinc-700 dark:text-zinc-300'}`}>
            {step.label}
          </span>
          <span className={`rounded-full px-1.5 py-0.5 text-[10px] font-semibold ${style.color} bg-white/50 dark:bg-slate-700/50`}>
            {style.label}
          </span>
          {step.status === 'active' && (
            <span className="text-[10px] text-sky-500 animate-pulse">执行中...</span>
          )}
        </div>

        {step.detail && (
          <p className="mt-1 text-[11px] text-zinc-500 dark:text-zinc-400 line-clamp-2">{step.detail}</p>
        )}

        {step.meta && step.meta.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {step.meta.slice(-2).map((item, i) => (
              <span key={i} className="inline-flex items-center gap-1 rounded-md bg-white/50 px-2 py-0.5 text-[10px] text-zinc-500 dark:bg-slate-700/50 dark:text-zinc-400">
                <span className="h-1 w-1 rounded-full bg-zinc-400" />
                <span className="max-w-[200px] truncate">{item}</span>
              </span>
            ))}
          </div>
        )}
      </div>

      {!isCompact && detail && (
        <span className="mt-1 shrink-0 text-[10px] text-zinc-300 opacity-0 transition-opacity group-hover:opacity-100 dark:text-zinc-600">
          详情 →
        </span>
      )}
    </motion.div>
  )
}
