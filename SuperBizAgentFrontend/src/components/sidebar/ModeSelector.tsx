import { motion } from 'framer-motion'
import { Zap, GitBranch } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  value: ChatMode
  onChange: (m: ChatMode) => void
}

const MODES: { id: ChatMode; label: string; icon: typeof Zap; desc: string }[] = [
  { id: 'quick', label: '快速回答', icon: Zap, desc: '一次性返回完整答案' },
  { id: 'stream', label: '流式输出', icon: GitBranch, desc: '边生成边展示' },
]

export function ModeSelector({ value, onChange }: Props) {
  return (
    <div className="glass rounded-xl p-3">
      <p className="text-xs text-zinc-500 mb-2">对话方式</p>
      <div className="flex gap-1 bg-zinc-950/50 rounded-lg p-1">
        {MODES.map((mode) => (
          <button
            key={mode.id}
            onClick={() => onChange(mode.id)}
            className={`relative flex-1 flex items-center justify-center gap-1.5 py-2 rounded-md text-xs transition-colors ${
              value === mode.id ? 'text-white' : 'text-zinc-500 hover:text-zinc-300'
            }`}
          >
            {value === mode.id && (
              <motion.div
                layoutId="mode-indicator"
                className="absolute inset-0 bg-accent/20 border border-accent/30 rounded-md"
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
