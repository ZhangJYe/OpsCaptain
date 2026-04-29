import { useState, useEffect } from 'react'
import { Search, MessageSquare, Trash2 } from 'lucide-react'
import { loadSessions, deleteSession } from '../../lib/storage'
import type { ChatSession } from '../../types/chat'

interface Props {
  onSelect: (s: ChatSession) => void
  currentSessionId: string
}

export function HistoryPanel({ onSelect, currentSessionId }: Props) {
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [search, setSearch] = useState('')

  useEffect(() => {
    setSessions(loadSessions())
  }, [])

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
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs text-zinc-500">历史会话</p>
        <span className="text-[10px] text-zinc-600">{filtered.length}</span>
      </div>

      <div className="relative mb-2">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-600" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="搜索历史会话..."
          className="w-full bg-zinc-950/50 rounded-lg pl-9 pr-3 py-2 text-xs outline-none placeholder:text-zinc-700 focus:ring-1 focus:ring-accent/50 transition-all"
        />
      </div>

      <div className="space-y-1 max-h-48 overflow-y-auto scrollbar-thin">
        {filtered.length === 0 ? (
          <p className="text-xs text-zinc-700 text-center py-4">暂无历史会话</p>
        ) : (
          filtered.map((s) => (
            <button
              key={s.id}
              onClick={() => onSelect(s)}
              className={`w-full flex items-start gap-2 p-2 rounded-lg text-left transition-colors group ${
                s.id === currentSessionId ? 'bg-accent/10 border border-accent/20' : 'hover:bg-zinc-800/50'
              }`}
            >
              <MessageSquare size={14} className="text-zinc-600 mt-0.5 shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-xs truncate">{s.title}</p>
                <p className="text-[10px] text-zinc-600">
                  {new Date(s.updatedAt).toLocaleDateString('zh-CN')}
                </p>
              </div>
              <button
                onClick={(e) => handleDelete(e, s.id)}
                className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-red-500/20 text-zinc-600 hover:text-red-400 transition-all"
              >
                <Trash2 size={12} />
              </button>
            </button>
          ))
        )}
      </div>
    </div>
  )
}
