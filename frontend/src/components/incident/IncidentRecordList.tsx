import { Activity, ArrowRight, Clock3, FileSearch, GitBranch, Loader2 } from 'lucide-react'
import { incidentEngineLabel, incidentRecordSummary, incidentStatusLabel, incidentUpdatedAt, sortedIncidentRecords } from '../../lib/incidentRecordSummary'
import type { IncidentSession } from '../../types/chat'

interface Props {
  incidents: IncidentSession[]
  isLoading: boolean
  onSelect: (incidentId: string) => void
}

function statusTone(status: IncidentSession['status']): string {
  switch (status) {
    case 'completed':
      return 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-300'
    case 'running':
      return 'bg-sky-500/10 text-sky-700 ring-sky-500/20 dark:text-sky-300'
    case 'waiting_approval':
      return 'bg-amber-500/10 text-amber-800 ring-amber-500/20 dark:text-amber-300'
    case 'degraded':
      return 'bg-orange-500/10 text-orange-700 ring-orange-500/20 dark:text-orange-300'
    case 'failed':
      return 'bg-rose-500/10 text-rose-700 ring-rose-500/20 dark:text-rose-300'
    default:
      return 'bg-zinc-500/10 text-zinc-600 ring-zinc-500/20 dark:text-zinc-300'
  }
}

export function IncidentRecordList({ incidents, isLoading, onSelect }: Props) {
  const records = sortedIncidentRecords(incidents)

  return (
    <div className="h-full overflow-y-auto bg-[#f7f8fa] dark:bg-[#09090b]">
      <div className="mx-auto max-w-5xl px-4 py-6 sm:py-8 lg:px-6">
        <div className="flex flex-wrap items-end justify-between gap-4 border-b border-zinc-200/80 pb-5 dark:border-zinc-800/80">
          <div>
            <div className="inline-flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
              <Activity size={14} className="text-accent" />
              事故诊断 <span className="text-zinc-300 dark:text-zinc-700">/</span> 记录库
            </div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-zinc-950 dark:text-white">事故记录</h1>
            <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">进入 Plan 或 GoS 的诊断会沉淀在这里，按最近更新时间排列。</p>
          </div>
          <div className="flex items-center gap-2 text-xs text-zinc-400 dark:text-zinc-500">
            {isLoading && <Loader2 size={14} className="animate-spin text-accent" />}
            <span>{records.length} 条记录</span>
          </div>
        </div>

        {records.length === 0 ? (
          <div className="flex min-h-[420px] flex-col items-center justify-center text-center">
            <span className="flex size-12 items-center justify-center rounded-2xl bg-sky-500/10 text-sky-600 dark:text-sky-300"><FileSearch size={22} /></span>
            <h2 className="mt-4 text-lg font-semibold text-zinc-950 dark:text-white">暂无事故记录</h2>
            <p className="mt-2 max-w-md text-sm leading-6 text-zinc-500 dark:text-zinc-400">请从工作台提交问题。只有进入 Plan 或 GoS 的诊断，才会在这里保存过程、证据与结论。</p>
          </div>
        ) : (
          <div className="mt-5 overflow-hidden border border-zinc-200/80 bg-white shadow-sm shadow-zinc-900/[0.025] dark:border-zinc-800/70 dark:bg-zinc-900/35">
            {records.map((incident) => (
              <button
                key={incident.incident_id}
                type="button"
                onClick={() => onSelect(incident.incident_id)}
                className="group grid w-full gap-3 border-b border-zinc-100 px-4 py-4 text-left transition hover:bg-sky-50/55 last:border-b-0 dark:border-zinc-800/70 dark:hover:bg-sky-950/20 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:px-5"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">{incident.title}</h2>
                    <span className={`inline-flex shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${statusTone(incident.status)}`}>
                      {incidentStatusLabel(incident.status)}
                    </span>
                  </div>
                  <p className="mt-2 line-clamp-2 text-sm leading-6 text-zinc-500 dark:text-zinc-400">{incidentRecordSummary(incident)}</p>
                  <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-zinc-400 dark:text-zinc-500">
                    <span className="inline-flex items-center gap-1"><GitBranch size={12} /> {incidentEngineLabel(incident.engine_strategy)}</span>
                    <span className="inline-flex items-center gap-1"><Clock3 size={12} /> 更新 {incidentUpdatedAt(incident.updated_at)}</span>
                  </div>
                </div>
                <span className="flex size-8 items-center justify-center self-end text-zinc-400 transition group-hover:translate-x-0.5 group-hover:text-sky-600 dark:text-zinc-600 dark:group-hover:text-sky-300 sm:self-center">
                  <ArrowRight size={17} />
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
