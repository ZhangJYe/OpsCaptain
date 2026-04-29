import { Menu, Moon, Sun, Layers } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  theme: string
  onToggleSidebar: () => void
  onToggleTheme: () => void
  chatMode: ChatMode
}

export function TopBar({ theme, onToggleSidebar, onToggleTheme, chatMode }: Props) {
  return (
    <header className="h-14 glass flex items-center justify-between px-4 shrink-0">
      <div className="flex items-center gap-3">
        <button
          onClick={onToggleSidebar}
          className="p-2 rounded-lg hover:bg-zinc-800 transition-colors"
          aria-label="切换侧栏"
        >
          <Menu size={20} />
        </button>
        <div className="flex items-center gap-2">
          <span className="w-7 h-7 rounded-md bg-accent flex items-center justify-center text-xs font-bold text-white">
            OC
          </span>
          <span className="font-semibold text-sm">OpsCaption</span>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <span className="text-xs px-2 py-1 rounded-full bg-accent/20 text-accent border border-accent/30">
          {chatMode === 'quick' ? '快速' : '流式'}
        </span>
        <button
          onClick={onToggleTheme}
          className="p-2 rounded-lg hover:bg-zinc-800 transition-colors"
          aria-label="切换主题"
        >
          {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
        </button>
      </div>
    </header>
  )
}
