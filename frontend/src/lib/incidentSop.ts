import type { AIOpsEngine, IncidentEvent, IncidentSession, IncidentStatus } from '../types/chat'

const statusLabel: Record<IncidentStatus, string> = {
  active: '可继续',
  running: '排障中',
  waiting_approval: '等待审批',
  completed: '已完成',
  degraded: '已降级',
  failed: '执行失败',
}

function engineLabel(engine: AIOpsEngine | string): string {
  return engine === 'gos_engine' || engine === 'gos' ? 'GoS' : 'Plan'
}

function formatDate(value?: number): string {
  if (!value) {
    return '未记录'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '未记录'
    : new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      }).format(date)
}

function completionTime(incident: IncidentSession): number | undefined {
  return [...incident.turns]
    .reverse()
    .find((turn) => turn.finished_at)?.finished_at
}

function unwrapProtocolContent(content: string): string {
  const start = content.indexOf('{')
  if (start < 0) {
    return content.trim()
  }
  try {
    const parsed: unknown = JSON.parse(content.slice(start))
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return content.trim()
    }
    const payload = parsed as Record<string, unknown>
    for (const key of ['response', 'answer', 'report']) {
      const value = payload[key]
      if (typeof value === 'string' && value.trim()) {
        return value.trim()
      }
    }
  } catch {
    return content.trim()
  }
  return content.trim()
}

function latestConclusion(incident: IncidentSession): string {
  for (let index = incident.turns.length - 1; index >= 0; index -= 1) {
    const result = incident.turns[index].result
    if (result?.trim()) {
      return unwrapProtocolContent(result)
    }
  }
  return unwrapProtocolContent(incident.latest_summary || '')
}

function suggestedAction(status: IncidentStatus, hasConclusion: boolean): string {
  if (status === 'waiting_approval') return '审批后继续写回当前事故；拒绝后仍可补充只读分析。'
  if (status === 'degraded') return '已保留当前证据，建议补充现象或切换到可用的数据源后继续。'
  if (status === 'failed') return '请补充影响范围与异常时间窗，再从当前事故继续排查。'
  if (hasConclusion) return '先复核结论引用的证据，再决定是否执行后续动作。'
  return '补充告警、日志、指标或影响范围，开始生成可复核的排障过程。'
}

function isKeyEvent(event: IncidentEvent): boolean {
  return !(event.type === 'task_info' && (event.message === 'llm_usage' || event.payload?.plan_detail === true))
}

function eventSummary(event: IncidentEvent): string {
  return event.message?.trim() || event.type.replace(/_/g, ' ')
}

function markdownList(items: string[], emptyText: string): string[] {
  return items.length > 0 ? items.map((item) => `- ${item}`) : [`- ${emptyText}`]
}

export function buildIncidentSop(incident: IncidentSession, exportedAt = new Date()): string {
  const conclusion = latestConclusion(incident)
  const evidence = incident.events.filter(isKeyEvent).slice(-6)
  const completedAt = completionTime(incident)
  const strategy = engineLabel(incident.engine_strategy)
  const action = suggestedAction(incident.status, Boolean(conclusion))

  return [
    `# ${incident.title || '未命名事故'} — SOP 草稿`,
    '',
    '> **待人工复核的 SOP 草稿**：本文件仅整理当前事故中的诊断过程与建议，不代表已批准或已执行的生产操作。',
    '',
    '## 事故元数据',
    `- 事故编号：${incident.incident_id}`,
    `- 当前状态：${statusLabel[incident.status]}`,
    `- 实际诊断策略：${strategy}`,
    `- 创建日期：${formatDate(incident.created_at)}`,
    `- 最近更新日期：${formatDate(incident.updated_at)}`,
    ...(completedAt ? [`- 完成日期：${formatDate(completedAt)}`] : []),
    `- 导出日期：${formatDate(exportedAt.getTime())}`,
    '',
    '## 根因结论',
    conclusion || '最终结论待补充。当前导出仅保留已产生的过程、证据与建议动作。',
    '',
    '## 可复核证据',
    ...markdownList(
      evidence.map((event) => `${formatDate(event.created_at)}｜${eventSummary(event)}`),
      '暂无可导出的事件证据。',
    ),
    '',
    '## 建议动作',
    `- ${action}`,
    '',
    '## 事故轮次',
    ...markdownList(
      incident.turns.map((turn, index) => `第 ${index + 1} 轮（${statusLabel[turn.status]}，${formatDate(turn.created_at)}）：${turn.user_query}`),
      '暂无事故轮次。',
    ),
    '',
    '## 复核记录',
    '- 请由值班或相关负责人核对上述结论与证据后，再补充最终处置步骤、责任人和执行记录。',
    '',
  ].join('\n')
}

export function incidentSopFilename(incident: IncidentSession, exportedAt = new Date()): string {
  const date = exportedAt.toISOString().slice(0, 10).replace(/-/g, '')
  const title = (incident.title || 'incident')
    .replace(/[^\w\u4e00-\u9fff-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 48)
  return `opscaptain-sop-${title || 'incident'}-${date}.md`
}
