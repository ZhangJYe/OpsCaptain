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
import remarkFixHeadings, { normalizeLooseMarkdown } from '../../lib/remarkFixHeadings'
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
    return summarizeProtocolMessage(event.message)
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

type ProcessStageStatus = 'pending' | 'active' | 'done' | 'attention'

interface ProcessStage {
  id: string
  title: string
  detail: string
  status: ProcessStageStatus
  updatedAt?: number
}

interface ProcessSignal {
  id: string
  turnId?: string
  label: string
  summary: string
  detail?: string
  createdAt: number
  tone: 'neutral' | 'evidence' | 'judgement' | 'attention' | 'done'
}

interface ProcessProjection {
  stages: ProcessStage[]
  signals: ProcessSignal[]
  currentStage: ProcessStage
  telemetryCount: number
  tokenCount: number
  meaningfulCount: number
}

function isGoSEngine(engine: AIOpsEngine | string): boolean {
  return engine === 'gos_engine' || engine === 'gos' || engine === 'aiops_gos_engine'
}

function stringPayload(event: IncidentEvent, key: string): string {
  const value = event.payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function numberPayload(event: IncidentEvent, key: string): number {
  const value = event.payload?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function isTelemetryEvent(event: IncidentEvent): boolean {
  return event.type === 'task_info' && event.message === 'llm_usage'
}

function isPlanDetailEvent(event: IncidentEvent): boolean {
  return event.type === 'task_info' && event.payload?.plan_detail === true
}

function stageTemplates(engine: AIOpsEngine | string): ProcessStage[] {
  if (isGoSEngine(engine)) {
    return [
      { id: 'hypothesis', title: '建立候选根因', detail: '从症状抽取故障方向。', status: 'pending' },
      { id: 'experts', title: '调度证据来源', detail: '选择日志、指标与知识检索路径。', status: 'pending' },
      { id: 'evidence', title: '挂载支持证据', detail: '保留支持、反驳和缺失证据。', status: 'pending' },
      { id: 'confidence', title: '收敛判断', detail: '更新候选方向和判断置信度。', status: 'pending' },
      { id: 'report', title: '输出结论', detail: '整理当前判断和后续建议。', status: 'pending' },
    ]
  }
  return [
    { id: 'symptom', title: '理解现象', detail: '读取本轮输入与事故上下文。', status: 'pending' },
    { id: 'plan', title: '制定计划', detail: '拆出排障步骤和检查目标。', status: 'pending' },
    { id: 'evidence', title: '执行检查', detail: '收集日志、指标与知识证据。', status: 'pending' },
    { id: 'report', title: '形成结论', detail: '整理判断、不确定性和建议。', status: 'pending' },
  ]
}

function updateStage(
  stages: ProcessStage[],
  stageId: string,
  status: ProcessStageStatus,
  detail: string,
  updatedAt: number,
) {
  const index = stages.findIndex((stage) => stage.id === stageId)
  if (index < 0) {
    return
  }
  stages.forEach((stage, stageIndex) => {
    if (stageIndex < index && (stage.status === 'pending' || stage.status === 'active')) {
      stage.status = 'done'
    }
  })
  const stage = stages[index]
  if (stage.status !== 'done' || status !== 'active') {
    stage.status = status
  }
  if (detail.trim()) {
    stage.detail = detail.trim()
  }
  stage.updatedAt = updatedAt
}

function completeAllStages(stages: ProcessStage[], detail: string, updatedAt: number) {
  stages.forEach((stage, index) => {
    stage.status = 'done'
    if (index === stages.length - 1 && detail.trim()) {
      stage.detail = detail.trim()
      stage.updatedAt = updatedAt
    }
  })
}

function markCurrentStage(stages: ProcessStage[], detail: string, updatedAt: number) {
  const current = [...stages].reverse().find((stage) => stage.status === 'attention')
    || [...stages].reverse().find((stage) => stage.status === 'active')
    || stages.find((stage) => stage.status === 'pending')
    || stages[stages.length - 1]
  current.status = 'attention'
  if (detail.trim()) {
    current.detail = detail.trim()
  }
  current.updatedAt = updatedAt
}

function addSignal(signals: ProcessSignal[], signal: ProcessSignal, seen: Set<string>) {
  const key = `${signal.turnId || 'incident'}:${signal.label}:${signal.summary}`
  if (!signal.summary.trim() || seen.has(key)) {
    return
  }
  seen.add(key)
  signals.push(signal)
}

function commonSignal(event: IncidentEvent): ProcessSignal | null {
  const summary = eventSummary(event)
  const degradation = stringPayload(event, 'degradation_reason')
  switch (event.type) {
    case 'approval_waiting':
      return { id: event.event_id, turnId: event.turn_id, label: '审批', summary, detail: '当前轮次已暂停。', createdAt: event.created_at, tone: 'attention' }
    case 'approval_executed':
      return { id: event.event_id, turnId: event.turn_id, label: '审批', summary, detail: '继续写回原事故。', createdAt: event.created_at, tone: 'done' }
    case 'approval_rejected':
      return { id: event.event_id, turnId: event.turn_id, label: '审批', summary, detail: stringPayload(event, 'review_reason'), createdAt: event.created_at, tone: 'attention' }
    case 'task_timeout':
    case 'task_failed':
    case 'turn_failed':
      return { id: event.event_id, turnId: event.turn_id, label: '异常', summary, detail: degradation, createdAt: event.created_at, tone: 'attention' }
    case 'turn_degraded':
      return { id: event.event_id, turnId: event.turn_id, label: '降级', summary, detail: degradation, createdAt: event.created_at, tone: 'attention' }
    case 'turn_completed':
      return { id: event.event_id, turnId: event.turn_id, label: '完成', summary, detail: '诊断结论已写回事故记录。', createdAt: event.created_at, tone: 'done' }
    default:
      return null
  }
}

function modelSignal(event: IncidentEvent): ProcessSignal | null {
  if (event.type !== 'task_info') {
    return null
  }
  switch (event.message) {
    case 'llm_queue_timeout':
      return { id: event.event_id, turnId: event.turn_id, label: '模型', summary: '模型调用排队超时，保留当前过程。', createdAt: event.created_at, tone: 'attention' }
    case 'llm_call_failed':
      return { id: event.event_id, turnId: event.turn_id, label: '模型', summary: '模型调用失败，当前轮次可能降级。', createdAt: event.created_at, tone: 'attention' }
    case 'llm_stream_failed':
      return { id: event.event_id, turnId: event.turn_id, label: '模型', summary: '模型流式响应失败，当前轮次可能降级。', createdAt: event.created_at, tone: 'attention' }
    default:
      return null
  }
}

function projectProcess(
  events: IncidentEvent[],
  engine: AIOpsEngine | string,
  incidentStatus: IncidentStatus,
): ProcessProjection {
  const stages = stageTemplates(engine)
  const signals: ProcessSignal[] = []
  const seenSignals = new Set<string>()
  let telemetryCount = 0
  let tokenCount = 0
  let meaningfulCount = 0

  events.forEach((event) => {
    if (isTelemetryEvent(event)) {
      telemetryCount += 1
      tokenCount += numberPayload(event, 'total_tokens')
      return
    }
    if (isPlanDetailEvent(event)) {
      return
    }

    meaningfulCount += 1
    const summary = eventSummary(event)
    const common = commonSignal(event)
    if (common) {
      addSignal(signals, common, seenSignals)
    }
    const model = modelSignal(event)
    if (model) {
      addSignal(signals, model, seenSignals)
      return
    }

    if (event.type === 'turn_started') {
      updateStage(
        stages,
        isGoSEngine(engine) ? 'hypothesis' : 'symptom',
        'active',
        summary,
        event.created_at,
      )
      return
    }

    if (event.type === 'approval_waiting') {
      markCurrentStage(stages, summary, event.created_at)
      return
    }

    if (event.type === 'turn_degraded' || event.type === 'turn_failed' || event.type === 'task_failed' || event.type === 'task_timeout') {
      markCurrentStage(stages, summary, event.created_at)
      return
    }

    if (event.type === 'turn_completed' || event.type === 'approval_executed') {
      completeAllStages(stages, summary, event.created_at)
      return
    }

    if (isGoSEngine(engine)) {
      const stage = stringPayload(event, 'stage')
      const stageMap: Record<string, { id: string; label: string; tone: ProcessSignal['tone'] }> = {
        ingest: { id: 'hypothesis', label: '候选', tone: 'neutral' },
        ingest_done: { id: 'hypothesis', label: '候选', tone: 'done' },
        frontier_selected: { id: 'experts', label: '方向', tone: 'judgement' },
        expert_planned: { id: 'experts', label: '调度', tone: 'neutral' },
        evidence_attached: { id: 'evidence', label: '证据', tone: 'evidence' },
        confidence_updated: { id: 'confidence', label: '判断', tone: 'judgement' },
        fsm_decision: { id: 'confidence', label: '收敛', tone: 'judgement' },
        report: { id: 'report', label: '报告', tone: 'done' },
      }
      const mapped = stageMap[stage]
      if (mapped) {
        updateStage(stages, mapped.id, stage === 'ingest_done' ? 'done' : 'active', summary, event.created_at)
        addSignal(
          signals,
          {
            id: event.event_id,
            turnId: event.turn_id,
            label: mapped.label,
            summary,
            detail: event.agent,
            createdAt: event.created_at,
            tone: mapped.tone,
          },
          seenSignals,
        )
      } else if (event.type === 'task_started') {
        updateStage(stages, 'hypothesis', 'active', 'GoS 已接管当前轮次。', event.created_at)
      } else if (event.type === 'task_completed') {
        updateStage(stages, 'report', 'active', summary, event.created_at)
      }
      return
    }

    if (event.type === 'task_started') {
      updateStage(stages, 'plan', 'active', 'Plan Runtime 已接管当前轮次。', event.created_at)
      return
    }

    if (event.type === 'task_info') {
      const stage = stringPayload(event, 'stage')
      const stageMap: Record<string, { id: string; label: string; tone: ProcessSignal['tone']; status: ProcessStageStatus }> = {
        planning: { id: 'plan', label: '计划', tone: 'neutral', status: 'active' },
        plan_ready: { id: 'plan', label: '计划', tone: 'done', status: 'done' },
        evidence_running: { id: 'evidence', label: '检查', tone: 'evidence', status: 'active' },
        evidence_ready: { id: 'evidence', label: '证据', tone: 'evidence', status: 'active' },
        report_ready: { id: 'report', label: '报告', tone: 'done', status: 'done' },
        report_degraded: { id: 'report', label: '报告', tone: 'attention', status: 'attention' },
        report_failed: { id: 'report', label: '报告', tone: 'attention', status: 'attention' },
      }
      const mapped = stageMap[stage]
      if (mapped) {
        updateStage(stages, mapped.id, mapped.status, summary, event.created_at)
        if (stage === 'plan_ready') {
          updateStage(stages, 'evidence', 'active', '按计划收集可验证证据。', event.created_at)
        }
        addSignal(
          signals,
          { id: event.event_id, turnId: event.turn_id, label: mapped.label, summary, detail: event.agent, createdAt: event.created_at, tone: mapped.tone },
          seenSignals,
        )
        return
      }
      if (summary.includes('排障步骤') || summary.includes('troubleshooting steps')) {
        updateStage(stages, 'plan', 'done', summary, event.created_at)
        updateStage(stages, 'evidence', 'active', '按计划收集可验证证据。', event.created_at)
        addSignal(
          signals,
          { id: event.event_id, turnId: event.turn_id, label: '计划', summary, detail: event.agent, createdAt: event.created_at, tone: 'neutral' },
          seenSignals,
        )
        return
      }
      if (summary.includes('诊断报告') || summary.includes('final diagnostic report')) {
        updateStage(stages, 'report', 'active', summary, event.created_at)
        addSignal(
          signals,
          { id: event.event_id, turnId: event.turn_id, label: '报告', summary, detail: event.agent, createdAt: event.created_at, tone: 'done' },
          seenSignals,
        )
        return
      }
      updateStage(stages, 'evidence', 'active', summary, event.created_at)
      addSignal(
        signals,
        { id: event.event_id, turnId: event.turn_id, label: '过程', summary, detail: event.agent, createdAt: event.created_at, tone: 'evidence' },
        seenSignals,
      )
      return
    }

    if (event.type === 'task_completed') {
      updateStage(stages, 'report', 'active', summary, event.created_at)
    }
  })

  const statusUpdatedAt = events[events.length - 1]?.created_at || Date.now()
  if (incidentStatus === 'completed') {
    completeAllStages(stages, stages[stages.length - 1].detail, stages[stages.length - 1].updatedAt || statusUpdatedAt)
  } else if (incidentStatus === 'waiting_approval') {
    markCurrentStage(stages, '排障暂停，等待审批继续执行。', statusUpdatedAt)
  } else if (incidentStatus === 'degraded' || incidentStatus === 'failed') {
    markCurrentStage(stages, incidentStatus === 'degraded' ? '本轮进入降级路径。' : '本轮执行失败。', statusUpdatedAt)
  }

  const currentStage = [...stages].reverse().find((stage) => stage.status === 'attention')
    || stages.find((stage) => stage.status === 'active')
    || [...stages].reverse().find((stage) => stage.status === 'done')
    || stages[0]

  return {
    stages,
    signals: signals.slice(-6),
    currentStage,
    telemetryCount,
    tokenCount,
    meaningfulCount,
  }
}

function processStageTone(stage: ProcessStage): string {
  switch (stage.status) {
    case 'done':
      return 'border-emerald-200 bg-emerald-500/10 text-emerald-700 dark:border-emerald-900/70 dark:text-emerald-300'
    case 'active':
      return 'border-sky-200 bg-sky-500/10 text-sky-700 dark:border-sky-900/70 dark:text-sky-300'
    case 'attention':
      return 'border-amber-200 bg-amber-500/10 text-amber-800 dark:border-amber-900/70 dark:text-amber-300'
    default:
      return 'border-zinc-200 bg-zinc-100/80 text-zinc-400 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-600'
  }
}

function processSignalTone(signal: ProcessSignal): string {
  switch (signal.tone) {
    case 'evidence':
      return 'bg-sky-500/10 text-sky-700 ring-sky-500/20 dark:text-sky-300'
    case 'judgement':
      return 'bg-indigo-500/10 text-indigo-700 ring-indigo-500/20 dark:text-indigo-300'
    case 'attention':
      return 'bg-amber-500/10 text-amber-800 ring-amber-500/20 dark:text-amber-300'
    case 'done':
      return 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-300'
    default:
      return 'bg-zinc-500/10 text-zinc-600 ring-zinc-500/20 dark:text-zinc-300'
  }
}

function rawEventLabel(event: IncidentEvent): string {
  if (isTelemetryEvent(event)) {
    return '模型用量'
  }
  switch (event.type) {
    case 'turn_started':
      return '轮次开始'
    case 'turn_completed':
      return '轮次完成'
    case 'turn_degraded':
      return '轮次降级'
    case 'turn_failed':
      return '轮次失败'
    case 'task_started':
      return '任务启动'
    case 'task_info':
      return '任务过程'
    case 'task_completed':
      return '任务完成'
    case 'task_timeout':
      return '任务超时'
    case 'task_failed':
      return '任务失败'
    case 'approval_waiting':
      return '等待审批'
    default:
      return event.type.replace(/_/g, ' ')
  }
}

function latestConclusion(incident: IncidentSession): string {
  for (let idx = incident.turns.length - 1; idx >= 0; idx -= 1) {
    if (incident.turns[idx].result?.trim()) {
      return unwrapProtocolContent(incident.turns[idx].result || '')
    }
  }
  return unwrapProtocolContent(incident.latest_summary || '')
}

function summarizeProtocolMessage(message: string): string {
  const generatedSteps = message.match(/^Plan generated (\d+) troubleshooting steps$/)
  if (generatedSteps) {
    return `Plan 已生成 ${generatedSteps[1]} 个排障步骤`
  }
  if (message === 'Plan generated troubleshooting steps') {
    return 'Plan 已生成排障步骤'
  }
  if (message === 'Plan generated final diagnostic report') {
    return 'Plan 已生成诊断报告'
  }
  const payload = parseProtocolPayload(message)
  if (Array.isArray(payload?.steps) || Array.isArray(payload?.plan)) {
    const count = Array.isArray(payload.steps) ? payload.steps.length : payload.plan.length
    return count > 0 ? `Plan 已生成 ${count} 个排障步骤` : 'Plan 已生成排障步骤'
  }
  if (typeof payload?.response === 'string' && payload.response.trim()) {
    return 'Plan 已生成诊断报告'
  }
  return message
}

function unwrapProtocolContent(content: string): string {
  const payload = parseProtocolPayload(content)
  for (const key of ['response', 'answer', 'report']) {
    if (typeof payload?.[key] === 'string' && payload[key].trim()) {
      return payload[key].trim()
    }
  }
  return content
}

function parseProtocolPayload(content: string): Record<string, any> | null {
  const start = content.indexOf('{')
  if (start < 0) {
    return null
  }
  try {
    const parsed = JSON.parse(content.slice(start))
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : null
  } catch {
    return null
  }
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

function suggestedAction(status: IncidentStatus, hasConclusion: boolean): string {
  if (status === 'waiting_approval') return '审批后继续写回当前事故；拒绝后仍可补充只读分析。'
  if (status === 'degraded') return '已保留当前证据，建议补充现象或切换到可用的数据源后继续。'
  if (status === 'failed') return '请补充影响范围与异常时间窗，再从当前事故继续排查。'
  if (hasConclusion) return '先复核结论引用的证据，再决定是否执行后续动作。'
  return '补充告警、日志、指标或影响范围，开始生成可复核的排障过程。'
}

export function IncidentView({ incident, isLoading, error, engine, onCreate, onAppend }: Props) {
  const [query, setQuery] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const status = statusMeta(incident?.status || 'active')
  const StatusIcon = status.icon
  const conclusion = incident ? latestConclusion(incident) : ''
  const recentEvents = useMemo(() => incident?.events.slice(-120) || [], [incident])
  const process = useMemo(
    () => projectProcess(recentEvents, incident?.engine_strategy || engine, incident?.status || 'active'),
    [engine, incident?.engine_strategy, incident?.status, recentEvents],
  )
  const evidenceCount = process.signals.filter((signal) => signal.tone === 'evidence' || signal.tone === 'judgement').length
  const action = suggestedAction(incident?.status || 'active', Boolean(conclusion))

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
    <div className="flex h-full min-h-0 flex-col bg-[#f7f8fa] dark:bg-[#09090b]">
      <div className="shrink-0 border-b border-zinc-200/80 bg-white/85 px-4 py-5 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/70 lg:px-6">
        <div className="mx-auto max-w-7xl">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <div className="mb-2 inline-flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
              <Activity size={14} className="text-accent" />
              事故诊断 <span className="text-zinc-300 dark:text-zinc-700">/</span> 当前事故
            </div>
            <h1 className="truncate text-2xl font-semibold tracking-tight text-zinc-950 dark:text-white">
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

          {incident && (
            <div className="mt-5 grid overflow-hidden rounded-2xl border border-zinc-200/80 bg-white shadow-sm shadow-zinc-900/[0.025] dark:border-zinc-800/70 dark:bg-zinc-900/45 sm:grid-cols-3">
              <div className="flex items-center gap-3 px-4 py-3.5 sm:border-r sm:border-zinc-100 dark:sm:border-zinc-800/70">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-sky-500/10 text-sky-600 dark:text-sky-300"><Activity size={17} /></span>
                <div><div className="text-xs text-zinc-500 dark:text-zinc-400">关键过程</div><div className="mt-0.5 text-sm font-semibold text-zinc-900 dark:text-white">{process.meaningfulCount} 条已沉淀</div></div>
              </div>
              <div className="flex items-center gap-3 border-t border-zinc-100 px-4 py-3.5 dark:border-zinc-800/70 sm:border-t-0 sm:border-r">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-600 dark:text-indigo-300"><FileSearch size={17} /></span>
                <div><div className="text-xs text-zinc-500 dark:text-zinc-400">证据与判断</div><div className="mt-0.5 text-sm font-semibold text-zinc-900 dark:text-white">{evidenceCount} 条可复核动态</div></div>
              </div>
              <div className="flex items-center gap-3 border-t border-zinc-100 px-4 py-3.5 dark:border-zinc-800/70 sm:border-t-0">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-300"><Clock3 size={17} /></span>
                <div><div className="text-xs text-zinc-500 dark:text-zinc-400">最近更新</div><div className="mt-0.5 text-sm font-semibold text-zinc-900 dark:text-white">{dateTime(incident.updated_at)}</div></div>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto scrollbar-thin">
        <div className="mx-auto grid max-w-7xl gap-5 px-4 py-5 lg:grid-cols-[minmax(0,1fr)_minmax(330px,0.9fr)] lg:px-6">
          <section className="min-w-0 rounded-2xl border border-zinc-200/80 bg-white p-4 shadow-sm shadow-zinc-900/[0.025] dark:border-zinc-800/70 dark:bg-zinc-900/35 sm:p-5">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">实时过程</h2>
                <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">
                只呈现可以回看和复核的排障主线。
                </p>
              </div>
              <span className="text-xs text-zinc-400 dark:text-zinc-600">{process.meaningfulCount} 关键事件</span>
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
              <div className="flex min-h-[300px] flex-col items-center justify-center rounded-xl border border-dashed border-zinc-300 px-6 text-center dark:border-zinc-800">
                <span className="flex size-11 items-center justify-center rounded-xl bg-sky-500/10 text-sky-600 dark:text-sky-300"><Activity size={20} /></span>
                <h3 className="mt-4 text-sm font-semibold text-zinc-800 dark:text-zinc-100">等待第一条现象</h3>
                <p className="mt-2 max-w-sm text-sm leading-6 text-zinc-500 dark:text-zinc-500">输入告警、日志、指标异常或影响范围后，这里会依次呈现过程、证据和结论。</p>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="overflow-hidden rounded-xl border border-zinc-200/80 bg-zinc-50/60 dark:border-zinc-800/70 dark:bg-zinc-950/20">
                  <div className="grid gap-4 border-b border-zinc-100 px-4 py-4 dark:border-zinc-800/80 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
                    <div className="min-w-0">
                      <div className="text-[11px] font-medium text-zinc-400 dark:text-zinc-500">当前阶段</div>
                      <div className="mt-1 flex items-center gap-2">
                        <span className={`inline-flex size-7 shrink-0 items-center justify-center rounded-full border ${processStageTone(process.currentStage)}`}>
                          {process.currentStage.status === 'done' ? (
                            <CheckCircle2 size={15} />
                          ) : process.currentStage.status === 'attention' ? (
                            <AlertTriangle size={15} />
                          ) : (
                            <Activity size={15} className={process.currentStage.status === 'active' ? 'animate-pulse' : ''} />
                          )}
                        </span>
                        <h3 className="truncate text-base font-semibold text-zinc-950 dark:text-white">{process.currentStage.title}</h3>
                      </div>
                      <p className="mt-2 text-sm leading-6 text-zinc-600 dark:text-zinc-300">{process.currentStage.detail}</p>
                    </div>
                    <div className="grid grid-cols-2 gap-px overflow-hidden bg-zinc-200/80 ring-1 ring-zinc-200/80 dark:bg-zinc-800 dark:ring-zinc-800 sm:min-w-[188px]">
                      <div className="bg-white px-3 py-2.5 dark:bg-zinc-950/70">
                        <div className="text-[11px] text-zinc-400 dark:text-zinc-500">过程</div>
                        <div className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">{process.meaningfulCount}</div>
                      </div>
                      <div className="bg-white px-3 py-2.5 dark:bg-zinc-950/70">
                        <div className="text-[11px] text-zinc-400 dark:text-zinc-500">遥测收纳</div>
                        <div className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">{process.telemetryCount}</div>
                      </div>
                    </div>
                  </div>

                  <div className="px-4 py-4">
                    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                      <h3 className="text-xs font-medium text-zinc-500 dark:text-zinc-400">排障阶段</h3>
                      <span className="text-[11px] text-zinc-400 dark:text-zinc-600">{engineLabel(incident?.engine_strategy || engine)} 策略</span>
                    </div>
                    <div className="space-y-3">
                      {process.stages.map((stage, index) => (
                        <div key={stage.id} className="grid grid-cols-[28px_minmax(0,1fr)] gap-3">
                          <div className="relative flex justify-center">
                            <span className={`relative z-10 inline-flex size-7 items-center justify-center rounded-full border ${processStageTone(stage)}`}>
                              {stage.status === 'done' ? (
                                <CheckCircle2 size={15} />
                              ) : stage.status === 'active' ? (
                                <Loader2 size={15} className="animate-spin" />
                              ) : stage.status === 'attention' ? (
                                <AlertTriangle size={15} />
                              ) : (
                                <Clock3 size={14} />
                              )}
                            </span>
                            {index < process.stages.length - 1 && (
                              <span className="absolute top-7 h-[calc(100%+12px)] w-px bg-zinc-200 dark:bg-zinc-800" />
                            )}
                          </div>
                          <div className="min-w-0 pb-1">
                            <div className="flex flex-wrap items-start justify-between gap-2">
                              <div className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{stage.title}</div>
                              {stage.updatedAt && <time className="text-[11px] text-zinc-400 dark:text-zinc-600">{dateTime(stage.updatedAt)}</time>}
                            </div>
                            <div className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">{stage.detail}</div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>

                <details className="group rounded-xl border border-zinc-200/80 bg-white/60 dark:border-zinc-800/70 dark:bg-zinc-900/25">
                  <summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm text-zinc-700 outline-none transition hover:bg-zinc-50 dark:text-zinc-200 dark:hover:bg-zinc-900/65">
                    <span className="font-medium">运行详情</span>
                    <span className="flex flex-wrap items-center gap-2 text-[11px] text-zinc-400 dark:text-zinc-500">
                      <span>{recentEvents.length} 原始事件</span>
                      {process.telemetryCount > 0 && <span>{process.telemetryCount} 次模型用量</span>}
                      {process.tokenCount > 0 && <span>{process.tokenCount} tokens</span>}
                      <span className="text-zinc-500 group-open:hidden dark:text-zinc-400">展开</span>
                      <span className="hidden text-zinc-500 group-open:inline dark:text-zinc-400">收起</span>
                    </span>
                  </summary>
                  <div className="space-y-2 border-t border-zinc-100 px-3 py-3 dark:border-zinc-800/80">
                    {recentEvents.map((event) => {
                      const meta = eventMeta(event)
                      return (
                        <div key={event.event_id} className="border border-zinc-200/70 bg-white/80 px-3 py-2.5 dark:border-zinc-800/70 dark:bg-zinc-950/45">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="flex flex-wrap items-center gap-2">
                                <span className="inline-flex rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400">
                                  {rawEventLabel(event)}
                                </span>
                                <span className="break-words text-xs text-zinc-700 dark:text-zinc-300">{eventSummary(event)}</span>
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
                </details>
              </div>
            )}
          </section>

          <section className="min-w-0 rounded-2xl border border-zinc-200/80 bg-white p-4 shadow-sm shadow-zinc-900/[0.025] dark:border-zinc-800/70 dark:bg-zinc-900/35 sm:p-5">
            <div className="mb-4 flex items-center gap-2">
              <FileSearch size={16} className="text-accent" />
              <div>
                <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">诊断结论</h2>
                <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">先给出判断，再保留每一条依据。</p>
              </div>
            </div>

            {error && (
              <div className="mb-4 rounded-xl border border-amber-200 bg-amber-50/80 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-200">
                <div className="flex gap-3">
                  <AlertTriangle size={17} className="mt-0.5 shrink-0" />
                  <div>
                    <div className="font-medium">暂时无法获取诊断结果</div>
                    <p className="mt-1 text-xs leading-5 opacity-80">当前已保留本地输入和过程；连接恢复后可继续在同一事故中排查。</p>
                    <details className="mt-2 text-xs opacity-70"><summary className="cursor-pointer">查看错误详情</summary><p className="mt-1 break-words">{error}</p></details>
                  </div>
                </div>
              </div>
            )}

            {conclusion ? (
              <div className="rounded-xl border border-sky-200/80 bg-sky-50/35 px-4 py-4 shadow-sm shadow-sky-500/[0.03] dark:border-sky-900/50 dark:bg-sky-950/15">
                <div className="prose-chat">
                  <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
                    {normalizeLooseMarkdown(conclusion)}
                  </ReactMarkdown>
                </div>
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-zinc-300 px-4 py-8 text-sm text-zinc-500 dark:border-zinc-800 dark:text-zinc-500">
                当前还没有最终结论；过程和证据会持续在这里汇总。
              </div>
            )}

            <div className="mt-5 rounded-xl border border-sky-200/80 bg-sky-50/45 px-4 py-4 dark:border-sky-900/50 dark:bg-sky-950/15">
              <div className="flex items-start gap-3">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-white text-sky-600 shadow-sm ring-1 ring-sky-100 dark:bg-slate-900 dark:text-sky-300 dark:ring-sky-900/60"><ArrowRight size={16} /></span>
                <div><h3 className="text-sm font-semibold text-zinc-900 dark:text-white">建议动作</h3><p className="mt-1 text-xs leading-5 text-zinc-600 dark:text-zinc-300">{action}</p></div>
              </div>
            </div>

            {process.signals.length > 0 && (
              <div className="mt-5">
                <div className="mb-2 flex items-center justify-between"><h3 className="text-xs font-medium text-zinc-500 dark:text-zinc-400">关键证据</h3><span className="text-[11px] text-zinc-400 dark:text-zinc-600">最近 {process.signals.length} 条</span></div>
                <div className="space-y-2">
                  {process.signals.map((signal) => (
                    <div key={signal.id} className="rounded-lg border border-zinc-200/80 bg-zinc-50/70 px-3 py-3 dark:border-zinc-800/70 dark:bg-zinc-950/20">
                      <div className="flex items-start gap-2"><span className={`mt-0.5 inline-flex shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${processSignalTone(signal)}`}>{signal.label}</span><div className="min-w-0 flex-1"><div className="break-words text-sm text-zinc-800 dark:text-zinc-200">{signal.summary}</div>{signal.detail && <div className="mt-1 text-[11px] text-zinc-400 dark:text-zinc-500">{signal.detail}</div>}</div></div>
                    </div>
                  ))}
                </div>
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
        <div className="mx-auto max-w-7xl rounded-xl border border-zinc-200/80 bg-white shadow-sm shadow-zinc-900/[0.02] dark:border-zinc-800/70 dark:bg-zinc-900/70">
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
