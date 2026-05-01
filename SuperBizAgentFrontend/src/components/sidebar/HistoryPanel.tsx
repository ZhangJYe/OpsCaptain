import { useState, useEffect } from 'react'
import { Search, MessageSquare, Trash2 } from 'lucide-react'
import { loadSessions, deleteSession } from '../../lib/storage'
import type { ChatSession } from '../../types/chat'

interface Props {
  onSelect: (s: ChatSession) => void
  currentSessionId: string
  messageCount: number
}

export function HistoryPanel({ onSelect, currentSessionId, messageCount }: Props) {
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [search, setSearch] = useState('')

  useEffect(() => {
    setSessions(loadSessions())
  }, [currentSessionId, messageCount])

  const filtered = sessions.filter(
    (s) => !search || s.title.toLowerCase().includes(search.toLowerCase())
  )

  const handleDelete = (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    deleteSession(id)
    setSessions((prev) => prev.filter((s) => s.id !== id))
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs text-zinc-600 dark:text-zinc-500">历史会话</p>
        <span className="text-[10px] text-zinc-500 dark:text-zinc-600">{filtered.length}</span>
      </div>

      <div className="relative mb-2">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500 dark:text-zinc-600" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="搜索历史会话..."
          className="w-full rounded-lg bg-zinc-100/90 py-2 pl-9 pr-3 text-xs text-zinc-900 outline-none transition-all placeholder:text-zinc-400 focus:ring-1 focus:ring-accent/50 dark:bg-zinc-950/50 dark:text-zinc-100 dark:placeholder:text-zinc-700"
        />
      </div>

      <div className="space-y-1 max-h-48 overflow-y-auto scrollbar-thin">
        {filtered.length === 0 ? (
          <p className="py-4 text-center text-xs text-zinc-400 dark:text-zinc-700">暂无历史会话</p>
        ) : (
          filtered.map((s) => (
            <div
              key={s.id}
              className={`group flex items-start gap-2 rounded-lg p-2 transition-colors ${
                s.id === currentSessionId
                  ? 'border border-accent/20 bg-accent/10'
                  : 'hover:bg-zinc-100 dark:hover:bg-zinc-800/50'
              }`}
            >
              <button onClick={() => onSelect(s)} className="flex min-w-0 flex-1 items-start gap-2 text-left">
                <MessageSquare size={14} className="mt-0.5 shrink-0 text-zinc-500 dark:text-zinc-600" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs">{s.title}</p>
                  <p className="text-[10px] text-zinc-500 dark:text-zinc-600">
                    {new Date(s.updatedAt).toLocaleDateString('zh-CN')}
                  </p>
                </div>
              </button>
              <button
                onClick={(e) => handleDelete(e, s.id)}
                className="rounded p-1 text-zinc-500 opacity-0 transition-all hover:bg-red-500/20 hover:text-red-400 group-hover:opacity-100 dark:text-zinc-600"
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
