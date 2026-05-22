import { useEffect, useState } from 'react'
import { Activity, MessageSquare, Search, Trash2 } from 'lucide-react'
import { deleteSession, loadSessions } from '../../lib/storage'
import type { ChatSession, IncidentSession } from '../../types/chat'

interface Props {
  onSelect: (session: ChatSession) => void
  onSelectIncident: (incidentId: string) => void
  currentSessionId: string
  currentIncidentId: string
  incidents: IncidentSession[]
  messageCount: number
}

export function HistoryPanel({
  onSelect,
  onSelectIncident,
  currentSessionId,
  currentIncidentId,
  incidents,
  messageCount,
}: Props) {
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [search, setSearch] = useState('')

  useEffect(() => {
    setSessions(loadSessions().filter((session) => session.workMode !== 'aiops'))
  }, [currentSessionId, messageCount])

  const filteredSessions = sessions.filter(
    (session) => !search || session.title.toLowerCase().includes(search.toLowerCase()),
  )
  const filteredIncidents = incidents.filter(
    (incident) => !search || incident.title.toLowerCase().includes(search.toLowerCase()),
  )

  const handleDelete = (event: React.MouseEvent, id: string) => {
    event.stopPropagation()
    deleteSession(id)
    setSessions((previous) => previous.filter((session) => session.id !== id))
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs font-medium text-zinc-700 dark:text-zinc-300">历史记录</p>
        <span className="text-[10px] text-zinc-500 dark:text-zinc-400">{filteredSessions.length + filteredIncidents.length}</span>
      </div>

      <div className="relative mb-3">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400 dark:text-zinc-500" />
        <input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="搜索问答或事故..."
          className="w-full rounded-lg bg-zinc-100/90 py-2 pl-9 pr-3 text-xs text-zinc-900 outline-none transition-all placeholder:text-zinc-400 focus:ring-1 focus:ring-accent/50 dark:bg-zinc-950/50 dark:text-zinc-100 dark:placeholder:text-zinc-700"
        />
      </div>

      <div className="mb-2 flex items-center justify-between">
        <p className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">问答历史</p>
        <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{filteredSessions.length}</span>
      </div>

      <div className="max-h-40 space-y-1 overflow-y-auto scrollbar-thin">
        {filteredSessions.length === 0 ? (
          <p className="py-3 text-center text-xs text-zinc-400 dark:text-zinc-700">暂无问答历史</p>
        ) : (
          filteredSessions.map((session) => (
            <div
              key={session.id}
              className={`group flex items-start gap-2 rounded-lg border p-2 transition-colors ${
                session.id === currentSessionId
                  ? 'border-sky-300/55 bg-sky-50/80 shadow-sm shadow-sky-500/5 dark:border-sky-500/25 dark:bg-sky-500/10'
                  : 'border-transparent hover:bg-zinc-100 dark:hover:bg-zinc-800/50'
              }`}
            >
              <button onClick={() => onSelect(session)} className="flex min-w-0 flex-1 items-start gap-2 text-left">
                <MessageSquare size={14} className={`mt-0.5 shrink-0 ${session.id === currentSessionId ? 'text-sky-500' : 'text-zinc-400 dark:text-zinc-500'}`} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-semibold text-zinc-800 dark:text-zinc-200">{session.title}</p>
                  <p className="text-[10px] text-zinc-400 dark:text-zinc-500">
                    {new Date(session.updatedAt).toLocaleDateString('zh-CN')}
                  </p>
                </div>
              </button>
              <button
                onClick={(event) => handleDelete(event, session.id)}
                className="rounded p-1 text-zinc-500 opacity-0 transition-all hover:bg-red-500/20 hover:text-red-400 group-hover:opacity-100 dark:text-zinc-600"
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))
        )}
      </div>

      <div className="mb-2 mt-3 flex items-center justify-between border-t border-zinc-200/80 pt-3 dark:border-zinc-800/70">
        <p className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">事故记录</p>
        <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{filteredIncidents.length}</span>
      </div>

      <div className="max-h-48 space-y-1 overflow-y-auto scrollbar-thin">
        {filteredIncidents.length === 0 ? (
          <p className="py-3 text-center text-xs text-zinc-400 dark:text-zinc-700">暂无事故记录</p>
        ) : (
          filteredIncidents.map((incident) => (
            <button
              key={incident.incident_id}
              onClick={() => onSelectIncident(incident.incident_id)}
              className={`flex w-full items-start gap-2 rounded-lg border p-2 text-left transition-colors ${
                incident.incident_id === currentIncidentId
                  ? 'border-sky-300/55 bg-sky-50/80 shadow-sm shadow-sky-500/5 dark:border-sky-500/25 dark:bg-sky-500/10'
                  : 'border-transparent hover:bg-zinc-100 dark:hover:bg-zinc-800/50'
              }`}
            >
              <Activity size={14} className="mt-0.5 shrink-0 text-accent" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-xs font-semibold text-zinc-800 dark:text-zinc-200">{incident.title}</p>
                <p className="mt-0.5 flex items-center justify-between gap-2 text-[10px] text-zinc-400 dark:text-zinc-600">
                  <span>{incident.status}</span>
                  <span>{new Date(incident.updated_at).toLocaleDateString('zh-CN')}</span>
                </p>
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  )
}
