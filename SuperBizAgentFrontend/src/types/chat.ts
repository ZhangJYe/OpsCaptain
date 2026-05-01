export type MessageRole = 'user' | 'assistant'

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  timestamp: number
}

export interface ChatSession {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
  mode?: ChatMode
  selectedSkillIds?: string[]
}

export type ChatMode = 'quick' | 'stream'

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
