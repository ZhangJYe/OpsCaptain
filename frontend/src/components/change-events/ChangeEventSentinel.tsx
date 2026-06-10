import { useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { Activity, Bell, BellRing, Check, ChevronUp, Clock3, RadioTower, Trash2, WifiOff, X } from 'lucide-react'
import { PetCharacter } from '../pet/PetCharacter'
import type { PetMood } from '../pet/PetCharacter'
import { useChangeEvents, type ChangeEventItem, type ChangeEventRisk, type ChangeEventStreamStatus } from '../../hooks/useChangeEvents'

const RISK_LABELS: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
  critical: '严重',
}

const TYPE_LABELS: Record<string, string> = {
  deploy: '发布',
  rollback: '回滚',
  git_push: '代码推送',
  pipeline: '流水线',
  config_update: '配置',
  scale: '扩缩容',
  restart: '重启',
  resource_update: '资源变更',
  dns_switch: '流量切换',
  failover: '故障切换',
  maintenance: '维护',
}

function riskTone(risk: ChangeEventRisk): string {
  switch (String(risk).toLowerCase()) {
    case 'critical':
      return 'border-rose-300 bg-rose-50 text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-300'
    case 'high':
      return 'border-orange-300 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300'
    case 'medium':
      return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
    default:
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
  }
}

function statusText(status: ChangeEventStreamStatus): string {
  switch (status) {
    case 'open':
      return '监听中'
    case 'connecting':
      return '连接中'
    case 'error':
      return '未连接'
    default:
      return '已关闭'
  }
}

function statusTone(status: ChangeEventStreamStatus): string {
  return status === 'open'
    ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.65)]'
    : status === 'connecting'
      ? 'bg-sky-400 shadow-[0_0_10px_rgba(56,189,248,0.65)] animate-pulse'
      : 'bg-zinc-300 dark:bg-zinc-600'
}

function eventMood(event: ChangeEventItem | undefined, status: ChangeEventStreamStatus): PetMood {
  if (status === 'connecting') return 'thinking'
  if (status === 'error') return 'error'
  const risk = String(event?.risk_level || '').toLowerCase()
  if (risk === 'critical' || risk === 'high') return 'error'
  if (event) return 'done'
  return 'idle'
}

function formatTime(value?: string): string {
  if (!value) return '--:--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function eventLabel(value: string, labels: Record<string, string>): string {
  return labels[value] || value.replace(/_/g, ' ')
}

function ChangeEventRow({ event }: { event: ChangeEventItem }) {
  const risk = String(event.risk_level || 'low').toLowerCase()
  return (
    <li className="group rounded-xl border border-white/65 bg-white/70 px-3 py-2.5 shadow-sm shadow-zinc-900/5 transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-slate-800/65 dark:hover:bg-slate-800">
      <div className="flex min-w-0 items-start gap-2.5">
        <span className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border ${riskTone(risk)}`}>
          <Activity size={14} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs font-semibold text-zinc-800 dark:text-zinc-100">{event.service}</span>
            <span className={`shrink-0 rounded-md border px-1.5 py-0.5 text-[10px] font-semibold ${riskTone(risk)}`}>
              {RISK_LABELS[risk] || risk}
            </span>
          </div>
          <p className="mt-1 line-clamp-2 text-[11px] leading-snug text-zinc-500 dark:text-zinc-400">
            {event.summary}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[10px] font-medium text-zinc-400 dark:text-zinc-500">
            <span className="rounded-md bg-zinc-100 px-1.5 py-0.5 dark:bg-slate-700/70">
              {eventLabel(event.event_type, TYPE_LABELS)}
            </span>
            <span>{event.env || 'unknown'}</span>
            <span className="text-zinc-300 dark:text-zinc-700">·</span>
            <span className="inline-flex items-center gap-1">
              <Clock3 size={10} />
              {formatTime(event.started_at)}
            </span>
          </div>
        </div>
      </div>
    </li>
  )
}

export function ChangeEventSentinel() {
  const { events, latestEvent, unreadCount, status, markRead, clear } = useChangeEvents()
  const [panelOpen, setPanelOpen] = useState(false)

  const mood = eventMood(latestEvent, status)
  const headline = useMemo(() => {
    if (latestEvent) {
      return `${latestEvent.service} ${eventLabel(latestEvent.event_type, TYPE_LABELS)}`
    }
    return status === 'open' ? '变更哨兵待命' : '等待事件流'
  }, [latestEvent, status])

  const handleToggle = () => {
    setPanelOpen((open) => {
      const next = !open
      if (next) markRead()
      return next
    })
  }

  return (
    <div className="pointer-events-none absolute bottom-[92px] right-3 z-40 flex flex-col items-end gap-2 sm:bottom-6 sm:right-5">
      <AnimatePresence>
        {!panelOpen && unreadCount > 0 && latestEvent && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.96 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 6, scale: 0.98 }}
            className="pointer-events-none max-w-[270px] rounded-2xl rounded-br-md border border-white/70 bg-white/95 px-3 py-2 text-right shadow-xl shadow-zinc-900/10 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/95"
          >
            <p className="truncate text-[12px] font-semibold text-zinc-800 dark:text-zinc-100">{headline}</p>
            <p className="mt-0.5 line-clamp-1 text-[11px] text-zinc-500 dark:text-zinc-400">{latestEvent.summary}</p>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {panelOpen && (
          <motion.aside
            initial={{ opacity: 0, y: 14, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 10, scale: 0.98 }}
            transition={{ type: 'spring', damping: 24, stiffness: 260 }}
            className="pointer-events-auto w-[min(360px,calc(100vw-24px))] overflow-hidden rounded-[20px] border border-white/70 bg-white/90 shadow-2xl shadow-zinc-900/10 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/95"
          >
            <div className="flex items-center justify-between border-b border-zinc-200/70 px-3.5 py-3 dark:border-white/10">
              <div className="flex min-w-0 items-center gap-2.5">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-sky-50 text-sky-600 ring-1 ring-sky-200/70 dark:bg-sky-500/10 dark:text-sky-300 dark:ring-sky-500/20">
                  <RadioTower size={16} />
                </span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-zinc-900 dark:text-white">变更哨兵</p>
                  <p className="mt-0.5 flex items-center gap-1.5 text-[11px] text-zinc-500 dark:text-zinc-400">
                    <span className={`h-1.5 w-1.5 rounded-full ${statusTone(status)}`} />
                    {statusText(status)}
                    <span className="text-zinc-300 dark:text-zinc-700">·</span>
                    {events.length} 条最近事件
                  </p>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  onClick={clear}
                  className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-slate-800 dark:hover:text-zinc-200"
                  aria-label="清空变更事件"
                  title="清空"
                >
                  <Trash2 size={14} />
                </button>
                <button
                  type="button"
                  onClick={handleToggle}
                  className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-slate-800 dark:hover:text-zinc-200"
                  aria-label="关闭变更事件面板"
                  title="关闭"
                >
                  <X size={15} />
                </button>
              </div>
            </div>

            {events.length > 0 ? (
              <ul className="max-h-[360px] space-y-2 overflow-y-auto p-3 scrollbar-thin">
                {events.map((event) => (
                  <ChangeEventRow key={event.event_id} event={event} />
                ))}
              </ul>
            ) : (
              <div className="px-4 py-8 text-center">
                <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-zinc-100 text-zinc-400 dark:bg-slate-800 dark:text-zinc-500">
                  {status === 'error' ? <WifiOff size={18} /> : <Check size={18} />}
                </div>
                <p className="mt-3 text-sm font-semibold text-zinc-700 dark:text-zinc-200">
                  {status === 'error' ? '事件流未连接' : '暂无变更事件'}
                </p>
                <p className="mt-1 text-xs leading-relaxed text-zinc-500 dark:text-zinc-500">
                  {status === 'error' ? '后端 SSE 不可用或鉴权未通过。' : '有发布、回滚或配置变更时会在这里出现。'}
                </p>
              </div>
            )}
          </motion.aside>
        )}
      </AnimatePresence>

      <button
        type="button"
        onClick={handleToggle}
        className="pointer-events-auto group flex items-center gap-2 rounded-[22px] border border-white/70 bg-white/90 px-2.5 py-2 shadow-xl shadow-zinc-900/10 backdrop-blur-xl transition-transform hover:scale-[1.02] focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/50 active:scale-95 dark:border-white/10 dark:bg-slate-900/90"
        aria-label={panelOpen ? '收起变更哨兵' : '打开变更哨兵'}
        title={panelOpen ? '收起变更哨兵' : '打开变更哨兵'}
      >
        <span className="relative">
          <PetCharacter mood={mood} size={48} className="rounded-[18px]" />
          <span className={`absolute -right-0.5 -top-0.5 h-3 w-3 rounded-full ring-2 ring-white dark:ring-slate-900 ${statusTone(status)}`} />
        </span>
        <span className="hidden min-w-0 text-left sm:block">
          <span className="block max-w-[150px] truncate text-xs font-semibold text-zinc-800 dark:text-zinc-100">
            {headline}
          </span>
          <span className="mt-0.5 flex items-center gap-1.5 text-[10px] font-medium text-zinc-500 dark:text-zinc-400">
            {unreadCount > 0 ? <BellRing size={11} /> : <Bell size={11} />}
            {unreadCount > 0 ? `${unreadCount} 条新变更` : statusText(status)}
          </span>
        </span>
        <span className="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-zinc-100 text-zinc-500 transition-colors group-hover:bg-sky-50 group-hover:text-sky-600 dark:bg-slate-800 dark:text-zinc-400 dark:group-hover:bg-sky-500/10 dark:group-hover:text-sky-300">
          {unreadCount > 0 && (
            <span className="absolute -right-1 -top-1 min-w-4 rounded-full bg-rose-500 px-1 text-[10px] font-bold leading-4 text-white">
              {unreadCount > 9 ? '9+' : unreadCount}
            </span>
          )}
          <ChevronUp size={15} className={panelOpen ? 'rotate-180 transition-transform' : 'transition-transform'} />
        </span>
      </button>
    </div>
  )
}
