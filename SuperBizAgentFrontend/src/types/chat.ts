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
}

export type ChatMode = 'quick' | 'stream'

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
