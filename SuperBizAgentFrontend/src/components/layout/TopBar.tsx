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
    <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b border-zinc-200/80 bg-white/90 px-4 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/90 lg:px-6">
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
          <div className="leading-none">
            <div className="text-sm font-semibold text-zinc-900 dark:text-white">OpsCaption</div>
            <div className="mt-1 text-[10px] uppercase tracking-[0.18em] text-zinc-400 dark:text-zinc-600">
              quiet ops workspace
            </div>
          </div>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <div className="hidden items-center gap-2 rounded-full border border-zinc-200/80 bg-zinc-50/90 px-3 py-1.5 text-[11px] text-zinc-500 dark:border-zinc-800/60 dark:bg-zinc-900/60 dark:text-zinc-400 md:flex">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          {chatMode === 'quick' ? 'direct answer' : 'streaming'}
        </div>
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium text-zinc-600 transition-colors hover:bg-zinc-100 hover:text-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
          aria-label="新建会话"
          title={isLoading ? '请等待当前请求完成' : '新建会话'}
        >
          <Plus size={15} />
          <span className="hidden sm:inline">新会话</span>
        </button>
        <span className="rounded-full bg-accent/10 px-2.5 py-1 text-[11px] font-medium text-accent md:hidden">
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
