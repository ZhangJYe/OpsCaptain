import { motion } from 'framer-motion'
import { X, FileText, BarChart3, ScrollText, BookOpen, AlertTriangle } from 'lucide-react'

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

const TYPE_CONFIG: Record<DetailItem['type'], { icon: typeof FileText; label: string; color: string }> = {
  metric: { icon: BarChart3, label: '指标', color: 'text-blue-500' },
  log: { icon: ScrollText, label: '日志', color: 'text-emerald-500' },
  knowledge: { icon: BookOpen, label: '知识库', color: 'text-violet-500' },
  error: { icon: AlertTriangle, label: '错误', color: 'text-red-500' },
  trace: { icon: FileText, label: 'Trace', color: 'text-amber-500' },
  info: { icon: FileText, label: '详情', color: 'text-zinc-500' },
}

export function DetailPanel({ item, onClose }: Props) {
  if (!item) return null

  const config = TYPE_CONFIG[item.type]
  const Icon = config.icon

  return (
    <motion.div
      initial={{ width: 0, opacity: 0 }}
      animate={{ width: 360, opacity: 1 }}
      exit={{ width: 0, opacity: 0 }}
      transition={{ type: 'spring', damping: 25, stiffness: 200 }}
      className="flex h-full flex-col overflow-hidden border-l border-zinc-200/80 bg-white/90 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/90"
    >
      <div className="flex items-center justify-between border-b border-zinc-200/80 px-4 py-3 dark:border-zinc-800/60">
        <div className="flex items-center gap-2">
          <Icon size={14} className={config.color} />
          <span className="text-xs font-medium text-zinc-500">{config.label}</span>
          <span className="text-xs text-zinc-400 dark:text-zinc-600 truncate max-w-[180px]">{item.title}</span>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
          aria-label="关闭详情"
        >
          <X size={14} />
        </button>
      </div>

      {item.meta && (
        <div className="border-b border-zinc-100 px-4 py-2 text-[11px] text-zinc-400 dark:border-zinc-800/40 dark:text-zinc-600">
          {item.meta}
        </div>
      )}

      <div className="flex-1 overflow-y-auto p-4">
        <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-zinc-700 dark:text-zinc-300">
          {item.content}
        </pre>
      </div>
    </motion.div>
  )
}
