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
    <header className="flex h-12 shrink-0 items-center justify-between gap-4 border-b border-white/40 bg-white/20 px-4 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/20 lg:px-6">
      <div className="flex items-center gap-3">
        <button
          onClick={onToggleSidebar}
          className="rounded-lg p-1.5 text-zinc-500 transition-colors hover:bg-white/50 hover:text-zinc-700 dark:hover:bg-slate-700/50 dark:hover:text-zinc-300"
          aria-label="切换侧栏"
        >
          <Menu size={16} />
        </button>
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-sky-500 text-[9px] font-bold text-white shadow-sm shadow-sky-400/25">
            OC
          </span>
          <div className="leading-none">
            <div className="text-[13px] font-semibold text-zinc-800 dark:text-white">OpsCaption</div>
            <div className="mt-0.5 text-[9px] uppercase tracking-[0.18em] text-zinc-400 dark:text-zinc-600">
              quiet ops workspace
            </div>
          </div>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <div className="hidden items-center gap-1.5 rounded-full border border-white/40 bg-white/30 px-2.5 py-1 text-[10px] font-medium text-zinc-500 backdrop-blur-sm dark:border-white/5 dark:bg-slate-700/30 dark:text-zinc-400 md:flex">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
          {chatMode === 'quick' ? 'direct answer' : 'streaming'}
        </div>
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium text-zinc-600 transition-all hover:-translate-y-0.5 hover:bg-white/50 hover:text-sky-600 hover:shadow-sm disabled:opacity-40 disabled:cursor-not-allowed dark:text-zinc-400 dark:hover:bg-slate-700/50 dark:hover:text-sky-400"
          aria-label="新建会话"
          title={isLoading ? '请等待当前请求完成' : '新建会话'}
        >
          <Plus size={14} />
          <span className="hidden sm:inline">新会话</span>
        </button>
        <span className="rounded-full bg-sky-400/15 px-2 py-0.5 text-[10px] font-semibold text-sky-600 dark:text-sky-400 md:hidden">
          {chatMode === 'quick' ? '快速' : '流式'}
        </span>
        <button
          onClick={onToggleTheme}
          className="rounded-lg p-1.5 text-zinc-500 transition-colors hover:bg-white/50 hover:text-zinc-700 dark:hover:bg-slate-700/50 dark:hover:text-zinc-300"
          aria-label="切换主题"
        >
          {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
        </button>
      </div>
    </header>
  )
}
