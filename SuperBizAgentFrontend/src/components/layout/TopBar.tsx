import { Menu, Moon, Sun, Plus } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  theme: string
  onToggleSidebar: () => void
  onToggleTheme: () => void
  chatMode: ChatMode
  onNewChat: () => void
  isLoading: boolean
}

export function TopBar({ theme, onToggleSidebar, onToggleTheme, chatMode, onNewChat, isLoading }: Props) {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b border-zinc-200/80 bg-white/88 px-4 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/88 lg:px-6">
      <div className="flex items-center gap-3">
        <button
          onClick={onToggleSidebar}
          className="rounded-lg p-2 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
          aria-label="切换侧栏"
        >
          <Menu size={18} />
        </button>
        <div className="flex items-center gap-2.5">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-accent text-[10px] font-bold text-white shadow-sm shadow-accent/20">
            OC
          </span>
          <span className="text-sm font-semibold text-zinc-900 dark:text-white">OpsCaption</span>
        </div>
      </div>

      <div className="flex items-center gap-1.5">
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="rounded-lg p-2 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
          aria-label="新建会话"
          title={isLoading ? '请等待当前请求完成' : '新建会话'}
        >
          <Plus size={18} />
        </button>
        <span className="rounded-full bg-accent/10 px-2.5 py-1 text-[11px] font-medium text-accent">
          {chatMode === 'quick' ? '快速' : '流式'}
        </span>
        <button
          onClick={onToggleTheme}
          className="rounded-lg p-2 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
          aria-label="切换主题"
        >
          {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
        </button>
      </div>
    </header>
  )
}
