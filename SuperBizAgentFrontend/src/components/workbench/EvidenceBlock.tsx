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
  bg: string
  border: string
  clickable: boolean
}

const BLOCK_STYLES: Record<string, BlockStyle> = {
  intent: { icon: Zap, label: '意图识别', color: 'text-zinc-400', bg: '', border: '', clickable: false },
  context: { icon: Layers, label: '上下文装配', color: 'text-zinc-400', bg: '', border: '', clickable: false },
  metrics: { icon: BarChart3, label: '指标证据', color: 'text-blue-500', bg: 'bg-blue-500/5', border: 'border-blue-200/60 dark:border-blue-800/40', clickable: true },
  logs: { icon: ScrollText, label: '日志证据', color: 'text-emerald-500', bg: 'bg-emerald-500/5', border: 'border-emerald-200/60 dark:border-emerald-800/40', clickable: true },
  knowledge: { icon: BookOpen, label: '知识库', color: 'text-violet-500', bg: 'bg-violet-500/5', border: 'border-violet-200/60 dark:border-violet-800/40', clickable: true },
  reporter: { icon: FileText, label: '生成报告', color: 'text-accent', bg: 'bg-accent/5', border: 'border-accent/20', clickable: false },
  contract: { icon: Shield, label: 'Contract 校验', color: 'text-amber-500', bg: 'bg-amber-500/5', border: 'border-amber-200/60 dark:border-amber-800/40', clickable: false },
  schema: { icon: Shield, label: 'Schema 校验', color: 'text-amber-500', bg: 'bg-amber-500/5', border: 'border-amber-200/60 dark:border-amber-800/40', clickable: false },
  'gos:hypothesis': { icon: Brain, label: '假设', color: 'text-accent', bg: 'bg-accent/5', border: 'border-accent/20', clickable: false },
  'gos:experts': { icon: Search, label: '专家调度', color: 'text-accent', bg: 'bg-accent/5', border: 'border-accent/20', clickable: false },
  'gos:evidence': { icon: BarChart3, label: '证据挂载', color: 'text-blue-500', bg: 'bg-blue-500/5', border: 'border-blue-200/60 dark:border-blue-800/40', clickable: true },
  'gos:confidence': { icon: Shield, label: '置信度', color: 'text-emerald-500', bg: 'bg-emerald-500/5', border: 'border-emerald-200/60 dark:border-emerald-800/40', clickable: false },
  'gos:reporter': { icon: FileText, label: 'GoS 报告', color: 'text-accent', bg: 'bg-accent/5', border: 'border-accent/20', clickable: false },
}

function getBlockStyle(stepId: string): BlockStyle {
  if (BLOCK_STYLES[stepId]) return BLOCK_STYLES[stepId]
  if (stepId.startsWith('tool:')) {
    return { icon: Zap, label: '工具调用', color: 'text-amber-500', bg: 'bg-amber-500/5', border: 'border-amber-200/60 dark:border-amber-800/40', clickable: true }
  }
  return { icon: FileText, label: '执行', color: 'text-zinc-400', bg: '', border: '', clickable: false }
}

function stepToDetail(step: ThinkingStep): DetailItem | null {
  if (!step.detail && (!step.meta || step.meta.length === 0)) return null
  const typeMap: Record<string, DetailItem['type']> = {
    metrics: 'metric', logs: 'log', knowledge: 'knowledge',
    'gos:evidence': 'metric',
  }
  return {
    id: step.id,
    type: typeMap[step.id] || (step.status === 'error' ? 'error' : 'info'),
    title: step.label,
    content: [step.detail, ...(step.meta || [])].filter(Boolean).join('\n\n'),
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
  const isCompact = !style.clickable && !style.bg
  const detail = stepToDetail(step)

  const handleClick = () => {
    if (style.clickable && detail && onOpenDetail) {
      onOpenDetail(detail)
    }
  }

  // Compact inline blocks (intent, context)
  if (isCompact) {
    return (
      <motion.div
        initial={{ opacity: 0, x: -8 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ type: 'spring', damping: 20, stiffness: 300 }}
        className="flex items-center gap-2 py-1"
      >
        {step.status === 'active' ? (
          <Loader2 size={11} className="shrink-0 animate-spin text-accent" />
        ) : step.status === 'done' ? (
          <Check size={11} className="shrink-0 text-emerald-400" />
        ) : step.status === 'error' ? (
          <AlertCircle size={11} className="shrink-0 text-red-400" />
        ) : (
          <Icon size={11} className={`shrink-0 ${style.color}`} />
        )}
        <span className="text-[11px] text-zinc-500 dark:text-zinc-500">{step.label}</span>
        {step.detail && (
          <span className="text-[11px] text-zinc-400 dark:text-zinc-600 truncate">{step.detail}</span>
        )}
      </motion.div>
    )
  }

  // Evidence / tool blocks (card style)
  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', damping: 20, stiffness: 300 }}
      className={`group flex items-start gap-3 rounded-xl border px-3 py-2.5 ${
        style.border || 'border-zinc-200/80 dark:border-zinc-800/60'
      } ${style.bg || 'bg-white/80 dark:bg-zinc-900/60'} ${
        style.clickable ? 'cursor-pointer transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40' : ''
      }`}
      onClick={handleClick}
      role={style.clickable ? 'button' : undefined}
      tabIndex={style.clickable ? 0 : undefined}
      onKeyDown={style.clickable ? (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleClick() }
      } : undefined}
    >
      {/* Status icon */}
      <div className="mt-0.5 shrink-0">
        {step.status === 'active' ? (
          <Loader2 size={14} className="animate-spin text-accent" />
        ) : step.status === 'done' ? (
          <Check size={14} className="text-emerald-400" />
        ) : step.status === 'error' ? (
          <AlertCircle size={14} className="text-red-400" />
        ) : (
          <Icon size={14} className={style.color} />
        )}
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className={`text-xs font-medium ${step.status === 'active' ? 'text-accent' : step.status === 'error' ? 'text-red-500' : 'text-zinc-700 dark:text-zinc-300'}`}>
            {step.label}
          </span>
          <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${style.color} ${style.bg}`}>
            {style.label}
          </span>
          {step.status === 'active' && (
            <span className="text-[10px] text-zinc-400 animate-pulse">执行中...</span>
          )}
        </div>

        {step.detail && (
          <p className="mt-1 text-[11px] text-zinc-500 dark:text-zinc-400 line-clamp-2">{step.detail}</p>
        )}

        {step.meta && step.meta.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {step.meta.slice(-2).map((item, i) => (
              <span key={i} className="inline-flex items-center gap-1 rounded-md bg-zinc-100 px-2 py-0.5 text-[10px] text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400">
                <span className="h-1 w-1 rounded-full bg-zinc-400" />
                <span className="max-w-[200px] truncate">{item}</span>
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Click hint */}
      {style.clickable && detail && (
        <span className="mt-1 shrink-0 text-[10px] text-zinc-300 opacity-0 transition-opacity group-hover:opacity-100 dark:text-zinc-600">
          详情 →
        </span>
      )}
    </motion.div>
  )
}
