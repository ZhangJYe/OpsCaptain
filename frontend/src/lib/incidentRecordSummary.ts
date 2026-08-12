import type { AIOpsEngine, IncidentSession, IncidentStatus } from '../types/chat'

const statusLabels: Record<IncidentStatus, string> = {
  active: '可继续',
  running: '排障中',
  waiting_approval: '等待审批',
  completed: '已完成',
  degraded: '已降级',
  failed: '执行失败',
}

export function incidentStatusLabel(status: IncidentStatus): string {
  return statusLabels[status]
}

export function incidentEngineLabel(engine: AIOpsEngine | string): string {
  return engine === 'gos_engine' || engine === 'gos' ? 'GoS' : 'Plan'
}

export function incidentUpdatedAt(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(value)
}

function firstText(content: string): string {
  return content
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#') && !/^[-|]+$/.test(line))
    .map((line) => line.replace(/^>\s*/, '').replace(/[*`|]/g, '').trim())
    .find(Boolean) || ''
}

export function incidentRecordSummary(incident: IncidentSession): string {
  const latestResult = incident.turns[incident.turns.length - 1]?.result || ''
  const summary = firstText(latestResult) || firstText(incident.latest_summary || '')
  if (summary) {
    return summary.length > 108 ? `${summary.slice(0, 108)}…` : summary
  }
  if (incident.status === 'running') return '诊断进行中，正在汇集过程与证据。'
  if (incident.status === 'waiting_approval') return '诊断已暂停，等待审批后继续。'
  return '暂未形成可展示的诊断结论。'
}

export function sortedIncidentRecords(incidents: IncidentSession[]): IncidentSession[] {
  return [...incidents].sort((left, right) => right.updated_at - left.updated_at)
}
