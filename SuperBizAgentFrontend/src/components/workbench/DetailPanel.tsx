import { motion } from 'framer-motion'
import { X, FileText, BarChart3, ScrollText, BookOpen, AlertTriangle, PanelRightClose } from 'lucide-react'

export interface DetailItem {
  id: string
  type: 'metric' | 'log' | 'knowledge' | 'error' | 'trace' | 'info'
  title: string
  content: string
  meta?: string
}

interface Props {
  item: DetailItem | null
  onClose: () => void
}

const TYPE_CONFIG: Record<DetailItem['type'], { icon: typeof FileText; label: string; dotColor: string; dotShadow: string }> = {
  metric: { icon: BarChart3, label: '指标', dotColor: 'bg-sky-400', dotShadow: 'shadow-[0_0_8px_rgba(56,189,248,0.5)]' },
  log: { icon: ScrollText, label: '日志', dotColor: 'bg-emerald-400', dotShadow: 'shadow-[0_0_8px_rgba(52,211,153,0.5)]' },
  knowledge: { icon: BookOpen, label: '知识库', dotColor: 'bg-violet-400', dotShadow: 'shadow-[0_0_8px_rgba(167,139,250,0.5)]' },
  error: { icon: AlertTriangle, label: '错误', dotColor: 'bg-rose-400', dotShadow: 'shadow-[0_0_8px_rgba(251,113,133,0.5)]' },
  trace: { icon: FileText, label: 'Trace', dotColor: 'bg-amber-400', dotShadow: 'shadow-[0_0_8px_rgba(251,191,36,0.5)]' },
  info: { icon: FileText, label: '详情', dotColor: 'bg-zinc-400', dotShadow: '' },
}

function EmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <PanelRightClose size={32} className="text-zinc-300 dark:text-zinc-700" />
      <p className="text-xs text-zinc-400 dark:text-zinc-600">
        点击左侧证据块查看详情
      </p>
      <p className="text-[10px] text-zinc-300 dark:text-zinc-700">
        指标、日志、工具调用等证据均可展开
      </p>
    </div>
  )
}

export function DetailPanel({ item, onClose }: Props) {
  const config = item ? TYPE_CONFIG[item.type] : null
  const Icon = config?.icon

  return (
    <motion.div
      initial={{ width: 0, opacity: 0 }}
      animate={{ width: 360, opacity: 1 }}
      exit={{ width: 0, opacity: 0 }}
      transition={{ type: 'spring', damping: 25, stiffness: 200 }}
      className="flex h-full flex-col overflow-hidden border-l border-white/40 bg-white/60 backdrop-blur-2xl dark:border-white/5 dark:bg-slate-800/40"
    >
      <div className="flex items-center justify-between border-b border-white/40 px-4 py-3 dark:border-white/5">
        {item && config ? (
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${config.dotColor} ${config.dotShadow}`} />
            <span className="text-xs font-semibold text-zinc-600 dark:text-zinc-400">{config.label}</span>
            <span className="text-xs text-zinc-400 dark:text-zinc-600 truncate max-w-[180px]">{item.title}</span>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold text-zinc-500 dark:text-zinc-500">证据详情</span>
          </div>
        )}
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg p-1 text-zinc-400 transition-colors hover:bg-white/50 hover:text-zinc-600 dark:hover:bg-slate-700/50 dark:hover:text-zinc-300"
          aria-label="关闭详情"
        >
          <X size={14} />
        </button>
      </div>

      {item ? (
        <>
          {item.meta && (
            <div className="border-b border-white/30 px-4 py-2 text-[11px] text-zinc-400 dark:border-white/5 dark:text-zinc-600">
              {item.meta}
            </div>
          )}
          <div className="flex-1 overflow-y-auto p-4">
            <pre className="whitespace-pre-wrap break-words rounded-xl border border-white/40 bg-white/40 p-3 font-mono text-xs leading-relaxed text-zinc-700 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/40 dark:text-zinc-300">
              {item.content}
            </pre>
          </div>
        </>
      ) : (
        <EmptyState />
      )}
    </motion.div>
  )
}
