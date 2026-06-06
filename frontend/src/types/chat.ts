export type MessageRole = 'user' | 'assistant'

export interface ChatExecutionStep {
  id: string
  label: string
  status: 'pending' | 'active' | 'done' | 'error'
  detail?: string
  meta?: string[]
}

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  timestamp: number
  executionSteps?: ChatExecutionStep[]
  engine?: string
  confidence?: number
  evidenceCount?: number
  nextActions?: string[]
  startedAt?: number
  finishedAt?: number
}

export interface ChatSession {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
  mode?: ChatMode
  workMode?: WorkMode
  selectedSkillIds?: string[]
}

export type ChatMode = 'quick' | 'stream'
export type WorkMode = 'react' | 'aiops'

export type WorkbenchMode = 'chat' | 'aiops' | 'settings'

export type AIOpsEngine = 'plan_execute_replan' | 'gos_engine'

export type IncidentStatus =
  | 'active'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'degraded'
  | 'failed'

export interface IncidentTurn {
  turn_id: string
  incident_id: string
  user_query: string
  trace_id?: string
  status: IncidentStatus
  result?: string
  detail?: string[]
  engine?: string
  approval_request_id?: string
  approval_status?: string
  degradation_reason?: string
  created_at: number
  finished_at?: number
}

export interface IncidentEvent {
  event_id: string
  incident_id: string
  turn_id?: string
  trace_id?: string
  type: string
  agent?: string
  message?: string
  payload?: Record<string, unknown>
  created_at: number
}

export interface IncidentSession {
  incident_id: string
  session_id: string
  title: string
  status: IncidentStatus
  engine_strategy: AIOpsEngine | string
  latest_summary?: string
  turns: IncidentTurn[]
  events: IncidentEvent[]
  created_at: number
  updated_at: number
}

export type SkillDomain = 'metrics' | 'logs' | 'knowledge'

export interface SkillOption {
  id: string
  label: string
  description: string
  domain: SkillDomain
  promptFocus: string
}

export interface SkillGroup {
  id: SkillDomain
  label: string
  description: string
  skills: SkillOption[]
}

export type OperatorName = '林澈' | '许知安' | '周望' | '陈序' | '沈宁' | '陆遥' | '顾川' | '叶岚'
export type OperatorTone = 'blue' | 'green' | 'amber' | 'slate'

export interface Operator {
  name: OperatorName
  tone: OperatorTone
}

export type ObservabilityStatus = 'healthy' | 'degraded' | 'down' | 'checking'
export interface EndpointStatus {
  name: string
  status: ObservabilityStatus
  text: string
  link: string
  lastCheck: number
}
