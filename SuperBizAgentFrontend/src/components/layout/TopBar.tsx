import { Menu, Moon, Sun, ShieldAlert } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  theme: string
  onToggleSidebar: () => void
  onToggleTheme: () => void
  chatMode: ChatMode
}

export function TopBar({ theme, onToggleSidebar, onToggleTheme, chatMode }: Props) {
  return (
    <header className="h-16 shrink-0 border-b border-zinc-200/80 bg-white/88 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/80">
      <div className="flex h-full items-center justify-between gap-4 px-4 lg:px-6">
        <div className="flex min-w-0 items-center gap-3">
        <button
          onClick={onToggleSidebar}
          className="rounded-xl border border-zinc-200/80 bg-white/80 p-2 text-zinc-600 transition-colors hover:bg-zinc-100 dark:border-zinc-800/80 dark:bg-zinc-900/70 dark:text-zinc-300 dark:hover:bg-zinc-800"
          aria-label="切换侧栏"
        >
          <Menu size={20} />
        </button>
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-2xl border border-accent/30 bg-accent/18 text-sm font-bold text-accent">
            OC
          </span>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">OpsCaption</div>
            <div className="truncate text-xs text-zinc-500 dark:text-zinc-500">生产诊断工作台</div>
          </div>
        </div>
      </div>

        <div className="flex items-center gap-2">
          <div className="hidden items-center gap-2 rounded-full border border-zinc-200/80 bg-zinc-100/90 px-3 py-1.5 text-xs text-zinc-500 dark:border-zinc-800/80 dark:bg-zinc-900/70 dark:text-zinc-400 md:flex">
            <ShieldAlert size={14} className="text-amber-400" />
            <span>生产环境</span>
          </div>
          <span className="rounded-full border border-accent/30 bg-accent/16 px-2.5 py-1 text-xs font-medium text-accent">
            {chatMode === 'quick' ? '快速回答' : '流式输出'}
          </span>
          <button
            onClick={onToggleTheme}
            className="rounded-xl border border-zinc-200/80 bg-white/80 p-2 text-zinc-600 transition-colors hover:bg-zinc-100 dark:border-zinc-800/80 dark:bg-zinc-900/70 dark:text-zinc-300 dark:hover:bg-zinc-800"
            aria-label="切换主题"
          >
            {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </button>
        </div>
      </div>
    </header>
  )
}
