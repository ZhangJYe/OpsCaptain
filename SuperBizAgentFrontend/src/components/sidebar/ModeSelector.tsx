import { motion } from 'framer-motion'
import { Zap, GitBranch } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  value: ChatMode
  onChange: (m: ChatMode) => void
}

const MODES: { id: ChatMode; label: string; icon: typeof Zap }[] = [
  { id: 'quick', label: '快速回答', icon: Zap },
  { id: 'stream', label: '流式输出', icon: GitBranch },
]

export function ModeSelector({ value, onChange }: Props) {
  return (
    <div className="rounded-xl border border-zinc-200/80 bg-white/80 p-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">对话方式</p>
      <div className="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
        {MODES.map((mode) => (
          <button
            key={mode.id}
            onClick={() => onChange(mode.id)}
            className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
              value === mode.id
                ? 'text-zinc-900 dark:text-white'
                : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
            }`}
          >
            {value === mode.id && (
              <motion.div
                layoutId="sidebar-mode"
                className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-zinc-200/60 dark:bg-zinc-700 dark:ring-zinc-600/60"
                transition={{ type: 'spring', damping: 20, stiffness: 300 }}
              />
            )}
            <mode.icon size={14} className="relative z-10" />
            <span className="relative z-10">{mode.label}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
