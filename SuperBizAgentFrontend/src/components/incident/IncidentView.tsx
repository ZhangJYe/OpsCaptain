import { useEffect, useMemo, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Clock3,
  FileSearch,
  GitBranch,
  Loader2,
  Send,
  ShieldAlert,
} from 'lucide-react'
import remarkFixHeadings from '../../lib/remarkFixHeadings'
import type { AIOpsEngine, IncidentEvent, IncidentSession, IncidentStatus, IncidentTurn } from '../../types/chat'

interface Props {
  incident: IncidentSession | null
  isLoading: boolean
  error: string | null
  engine: AIOpsEngine
  onCreate: (query: string) => void
  onAppend: (query: string) => void
}

function engineLabel(engine: AIOpsEngine | string): string {
  return engine === 'gos_engine' || engine === 'gos' ? 'GoS' : 'Plan'
}

function statusMeta(status: IncidentStatus) {
  switch (status) {
    case 'running':
      return { label: '排障中', tone: 'bg-sky-500/10 text-sky-600 ring-sky-500/20 dark:text-sky-300', icon: Loader2 }
    case 'waiting_approval':
      return { label: '等待审批', tone: 'bg-amber-500/10 text-amber-700 ring-amber-500/20 dark:text-amber-300', icon: ShieldAlert }
    case 'completed':
      return { label: '已完成', tone: 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-300', icon: CheckCircle2 }
    case 'degraded':
      return { label: '已降级', tone: 'bg-orange-500/10 text-orange-700 ring-orange-500/20 dark:text-orange-300', icon: AlertTriangle }
    case 'failed':
      return { label: '执行失败', tone: 'bg-rose-500/10 text-rose-700 ring-rose-500/20 dark:text-rose-300', icon: AlertTriangle }
    default:
      return { label: '可继续', tone: 'bg-zinc-500/10 text-zinc-600 ring-zinc-500/20 dark:text-zinc-300', icon: Clock3 }
  }
}

function dateTime(value?: number): string {
  if (!value) {
    return '--'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(value)
}

function eventSummary(event: IncidentEvent): string {
  if (event.message) {
    return event.message
  }
  return event.type.replace(/_/g, ' ')
}

function eventMeta(event: IncidentEvent): string[] {
  const payload = event.payload || {}
  return [
    event.agent,
    event.trace_id ? `trace ${event.trace_id}` : '',
    typeof payload.status === 'string' ? payload.status : '',
    typeof payload.degradation_reason === 'string' ? payload.degradation_reason : '',
  ].filter(Boolean) as string[]
}

function latestConclusion(incident: IncidentSession): string {
  for (let idx = incident.turns.length - 1; idx >= 0; idx -= 1) {
    if (incident.turns[idx].result?.trim()) {
      return incident.turns[idx].result || ''
    }
  }
  return incident.latest_summary || ''
}

function terminalTone(turn: IncidentTurn): string {
  if (turn.status === 'degraded') {
    return 'text-orange-600 dark:text-orange-300'
  }
  if (turn.status === 'failed') {
    return 'text-rose-600 dark:text-rose-300'
  }
  if (turn.status === 'waiting_approval') {
    return 'text-amber-600 dark:text-amber-300'
  }
  return 'text-zinc-500 dark:text-zinc-400'
}

export function IncidentView({ incident, isLoading, error, engine, onCreate, onAppend }: Props) {
  const [query, setQuery] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const status = statusMeta(incident?.status || 'active')
  const StatusIcon = status.icon
  const conclusion = incident ? latestConclusion(incident) : ''
  const recentEvents = useMemo(() => incident?.events.slice(-120) || [], [incident])

  useEffect(() => {
    if (!textareaRef.current) {
      return
    }
    textareaRef.current.style.height = 'auto'
    textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 164)}px`
  }, [query])

  const submit = () => {
    const trimmed = query.trim()
    if (!trimmed || isLoading) {
      return
    }
    if (incident) {
      onAppend(trimmed)
    } else {
      onCreate(trimmed)
    }
    setQuery('')
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#fafafa] dark:bg-[#09090b]">
      <div className="shrink-0 border-b border-zinc-200/80 bg-white/80 px-4 py-4 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/70 lg:px-6">
        <div className="mx-auto flex max-w-6xl flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <div className="mb-2 inline-flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
              <Activity size={14} className="text-accent" />
              事故排障
            </div>
            <h1 className="truncate text-xl font-semibold text-zinc-950 dark:text-white">
              {incident?.title || '描述首条现象，创建事故记录'}
            </h1>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 ring-1 ring-inset ${status.tone}`}>
              <StatusIcon size={13} className={incident?.status === 'running' ? 'animate-spin' : ''} />
              {status.label}
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-zinc-100 px-2.5 py-1 text-zinc-600 ring-1 ring-inset ring-zinc-200 dark:bg-zinc-900 dark:text-zinc-300 dark:ring-zinc-800">
              <GitBranch size={13} />
              {engineLabel(incident?.engine_strategy || engine)}
            </span>
            {incident && (
              <span className="text-zinc-400 dark:text-zinc-500">
                更新 {dateTime(incident.updated_at)}
              </span>
            )}
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto scrollbar-thin">
        <div className="mx-auto grid max-w-6xl gap-0 px-4 py-5 lg:grid-cols-[minmax(0,1fr)_minmax(320px,0.92fr)] lg:px-6">
          <section className="min-w-0 border-b border-zinc-200/80 pb-5 lg:border-b-0 lg:border-r lg:pr-6 dark:border-zinc-800/70">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">实时过程</h2>
                <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">
                  阶段、证据、降级和审批都沉淀在当前事故。
                </p>
              </div>
              <span className="text-xs text-zinc-400 dark:text-zinc-600">{recentEvents.length} 事件</span>
            </div>

            {incident?.status === 'waiting_approval' && (
              <div className="mb-4 flex gap-3 border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
                <ShieldAlert size={18} className="mt-0.5 shrink-0" />
                <div>
                  <div className="font-medium">已暂停，等待审批</div>
                  <div className="mt-1 text-xs opacity-80">审批后继续写回同一事故；拒绝后仍可追加只读分析。</div>
                </div>
              </div>
            )}

            {(incident?.status === 'degraded' || incident?.status === 'failed') && (
              <div className="mb-4 flex gap-3 border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-900 dark:border-orange-900/60 dark:bg-orange-950/25 dark:text-orange-200">
                <AlertTriangle size={18} className="mt-0.5 shrink-0" />
                <div>
                  <div className="font-medium">{incident.status === 'degraded' ? '本轮已降级' : '本轮执行失败'}</div>
                  <div className="mt-1 text-xs opacity-80">已拿到的事件和证据会保留，可继续补充现象。</div>
                </div>
              </div>
            )}

            {recentEvents.length === 0 ? (
              <div className="flex min-h-[220px] items-center justify-center border border-dashed border-zinc-300 px-6 text-center text-sm text-zinc-500 dark:border-zinc-800 dark:text-zinc-500">
                输入首条现象后，这里会显示排障阶段和证据回放。
              </div>
            ) : (
              <div className="space-y-2">
                {recentEvents.map((event) => {
                  const meta = eventMeta(event)
                  return (
                    <div
                      key={event.event_id}
                      className="border border-zinc-200/80 bg-white/80 px-3 py-3 dark:border-zinc-800/70 dark:bg-zinc-900/45"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="inline-flex rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-medium uppercase text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400">
                              {event.type}
                            </span>
                            <span className="text-sm text-zinc-800 dark:text-zinc-200">{eventSummary(event)}</span>
                          </div>
                          {meta.length > 0 && (
                            <div className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-[11px] text-zinc-400 dark:text-zinc-500">
                              {meta.map((item) => (
                                <span key={item}>{item}</span>
                              ))}
                            </div>
                          )}
                        </div>
                        <time className="shrink-0 text-[11px] text-zinc-400 dark:text-zinc-600">{dateTime(event.created_at)}</time>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </section>

          <section className="min-w-0 pt-5 lg:pl-6 lg:pt-0">
            <div className="mb-4 flex items-center gap-2">
              <FileSearch size={16} className="text-accent" />
              <div>
                <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">诊断结论</h2>
                <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">保留轮次、trace 和最新判断。</p>
              </div>
            </div>

            {error && (
              <div className="mb-4 border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/25 dark:text-rose-200">
                {error}
              </div>
            )}

            {conclusion ? (
              <div className="border border-zinc-200/80 bg-white/90 px-4 py-4 shadow-sm shadow-zinc-900/[0.03] dark:border-zinc-800/70 dark:bg-zinc-900/55">
                <div className="prose-chat">
                  <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>{conclusion}</ReactMarkdown>
                </div>
              </div>
            ) : (
              <div className="border border-dashed border-zinc-300 px-4 py-8 text-sm text-zinc-500 dark:border-zinc-800 dark:text-zinc-500">
                当前还没有最终结论。
              </div>
            )}

            {incident && (
              <div className="mt-5">
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-xs font-medium text-zinc-500 dark:text-zinc-400">事故轮次</h3>
                  <span className="text-[11px] text-zinc-400 dark:text-zinc-600">{incident.turns.length}</span>
                </div>
                <div className="space-y-2">
                  {incident.turns.map((turn, index) => (
                    <div key={turn.turn_id} className="border border-zinc-200/80 bg-white/70 px-3 py-3 dark:border-zinc-800/70 dark:bg-zinc-900/35">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="text-[11px] text-zinc-400 dark:text-zinc-600">第 {index + 1} 轮</div>
                          <div className="mt-1 break-words text-sm text-zinc-800 dark:text-zinc-200">{turn.user_query}</div>
                        </div>
                        <span className={`shrink-0 text-[11px] ${terminalTone(turn)}`}>{statusMeta(turn.status).label}</span>
                      </div>
                      <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-zinc-400 dark:text-zinc-600">
                        <span>{dateTime(turn.created_at)}</span>
                        {turn.trace_id && <span>trace {turn.trace_id}</span>}
                        {turn.approval_request_id && <span>approval {turn.approval_request_id}</span>}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </section>
        </div>
      </div>

      <div className="shrink-0 border-t border-zinc-200/80 bg-white/90 px-4 py-4 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/88 lg:px-6">
        <div className="mx-auto max-w-6xl border border-zinc-200/80 bg-white dark:border-zinc-800/70 dark:bg-zinc-900/70">
          <textarea
            ref={textareaRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={handleKeyDown}
            rows={1}
            placeholder={incident ? '补充现象、证据或追问下一步...' : '描述告警、日志、指标异常或影响范围...'}
            className="min-h-[54px] w-full resize-none bg-transparent px-4 py-3 text-sm leading-7 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
          />
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-zinc-100 px-3 py-2.5 dark:border-zinc-800">
            <div className="flex min-w-0 items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
              {isLoading ? <Loader2 size={14} className="animate-spin text-accent" /> : <ArrowRight size={14} className="text-accent" />}
              <span className="truncate">
                {incident ? '后续输入追加到当前事故' : `输入即创建事故，默认 ${engineLabel(engine)} 策略`}
              </span>
            </div>
            <button
              onClick={submit}
              disabled={!query.trim() || isLoading}
              className="inline-flex h-9 items-center justify-center gap-2 bg-accent px-4 text-sm font-medium text-white transition hover:brightness-110 disabled:cursor-not-allowed disabled:bg-zinc-100 disabled:text-zinc-400 dark:disabled:bg-zinc-800 dark:disabled:text-zinc-600"
            >
              <Send size={14} />
              {incident ? '追加排障' : '创建事故'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
