import { Activity, Check, Clock3, RadioTower, Trash2, WifiOff, X } from 'lucide-react'
import type { ChangeEventItem, ChangeEventStreamStatus } from '../../hooks/useChangeEvents'
import {
  changeEventRiskLabel,
  changeEventRiskTone,
  changeEventStreamLabel,
  changeEventStreamTone,
  changeEventTypeLabel,
} from '../../lib/changeEventPresentation'

interface Props {
  events: ChangeEventItem[]
  status: ChangeEventStreamStatus
  onClear: () => void
  onClose: () => void
}

function formatTime(value?: string): string {
  if (!value) return '--:--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '--:--' : date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function ChangeEventRow({ event }: { event: ChangeEventItem }) {
  const risk = String(event.risk_level || 'low').toLowerCase()
  return (
    <li className="group rounded-xl border border-white/65 bg-white/70 px-3 py-2.5 shadow-sm shadow-zinc-900/5 transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-slate-800/65 dark:hover:bg-slate-800">
      <div className="flex min-w-0 items-start gap-2.5">
        <span className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border ${changeEventRiskTone(risk)}`}>
          <Activity size={14} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs font-semibold text-zinc-800 dark:text-zinc-100">{event.service}</span>
            <span className={`shrink-0 rounded-md border px-1.5 py-0.5 text-[10px] font-semibold ${changeEventRiskTone(risk)}`}>
              {changeEventRiskLabel(risk)}
            </span>
          </div>
          <p className="mt-1 line-clamp-2 text-[11px] leading-snug text-zinc-500 dark:text-zinc-400">{event.summary}</p>
          <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[10px] font-medium text-zinc-400 dark:text-zinc-500">
            <span className="rounded-md bg-zinc-100 px-1.5 py-0.5 dark:bg-slate-700/70">{changeEventTypeLabel(event.event_type)}</span>
            <span>{event.env || 'unknown'}</span>
            <span className="text-zinc-300 dark:text-zinc-700">·</span>
            <span className="inline-flex items-center gap-1"><Clock3 size={10} />{formatTime(event.started_at)}</span>
          </div>
        </div>
      </div>
    </li>
  )
}

export function ChangeEventPanel({ events, status, onClear, onClose }: Props) {
  return (
    <aside className="w-[min(360px,calc(100vw-24px))] overflow-hidden rounded-[20px] border border-white/70 bg-white/90 shadow-2xl shadow-zinc-900/10 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/95">
      <div className="flex items-center justify-between border-b border-zinc-200/70 px-3.5 py-3 dark:border-white/10">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-sky-50 text-sky-600 ring-1 ring-sky-200/70 dark:bg-sky-500/10 dark:text-sky-300 dark:ring-sky-500/20"><RadioTower size={16} /></span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-zinc-900 dark:text-white">变更哨兵</p>
            <p className="mt-0.5 flex items-center gap-1.5 text-[11px] text-zinc-500 dark:text-zinc-400">
              <span className={`h-1.5 w-1.5 rounded-full ${changeEventStreamTone(status)}`} />
              {changeEventStreamLabel(status)}
              <span className="text-zinc-300 dark:text-zinc-700">·</span>
              {events.length} 条最近事件
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button type="button" onClick={onClear} className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-slate-800 dark:hover:text-zinc-200" aria-label="清空变更事件" title="清空"><Trash2 size={14} /></button>
          <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-slate-800 dark:hover:text-zinc-200" aria-label="关闭变更事件面板" title="关闭"><X size={15} /></button>
        </div>
      </div>

      {events.length > 0 ? (
        <ul className="max-h-[360px] space-y-2 overflow-y-auto p-3 scrollbar-thin">
          {events.map((event) => <ChangeEventRow key={event.event_id} event={event} />)}
        </ul>
      ) : (
        <div className="px-4 py-8 text-center">
          <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-zinc-100 text-zinc-400 dark:bg-slate-800 dark:text-zinc-500">{status === 'error' ? <WifiOff size={18} /> : <Check size={18} />}</div>
          <p className="mt-3 text-sm font-semibold text-zinc-700 dark:text-zinc-200">{status === 'error' ? '事件流未连接' : '暂无变更事件'}</p>
          <p className="mt-1 text-xs leading-relaxed text-zinc-500 dark:text-zinc-500">{status === 'error' ? '后端 SSE 不可用或鉴权未通过。' : '有发布、回滚或配置变更时会在这里出现。'}</p>
        </div>
      )}
    </aside>
  )
}
