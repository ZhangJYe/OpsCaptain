import type { ChatMessage, ChatMode } from '../types/chat'

const STORAGE_KEY = 'opscaption-chat-history'
const LEGACY_STORAGE_KEY = 'chatHistories'
const MAX_HISTORY = 50

interface StoredSession {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
  mode?: ChatMode
  selectedSkillIds?: string[]
}

function parseTimestamp(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  const parsed = Date.parse(String(value || ''))
  return Number.isFinite(parsed) ? parsed : Date.now()
}

export function loadSessions(): StoredSession[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY)
    const parsed = raw ? JSON.parse(raw) : []
    if (!Array.isArray(parsed)) {
      return []
    }
    return parsed.map((session) => ({
      id: String(session.id || ''),
      title: String(session.title || '新会话'),
      messages: Array.isArray(session.messages) ? session.messages : [],
      createdAt: parseTimestamp(session.createdAt),
      updatedAt: parseTimestamp(session.updatedAt),
      mode: session.mode === 'stream' ? 'stream' : 'quick',
      selectedSkillIds: Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [],
    }))
  } catch {
    return []
  }
}

interface SaveSessionOptions {
  mode?: ChatMode
  selectedSkillIds?: string[]
}

export function saveSession(id: string, messages: ChatMessage[], options: SaveSessionOptions = {}) {
  const sessions = loadSessions()
  const idx = sessions.findIndex((s) => s.id === id)
  const title = messages[0]?.content?.slice(0, 50) || '新会话'
  const session: StoredSession = {
    id,
    title,
    messages: messages.slice(-50),
    createdAt: idx >= 0 ? sessions[idx].createdAt : Date.now(),
    updatedAt: Date.now(),
    mode: options.mode,
    selectedSkillIds: Array.isArray(options.selectedSkillIds) ? options.selectedSkillIds : [],
  }
  if (idx >= 0) {
    sessions[idx] = session
  } else {
    sessions.unshift(session)
  }
  if (sessions.length > MAX_HISTORY) {
    sessions.length = MAX_HISTORY
  }
  const payload = JSON.stringify(sessions)
  localStorage.setItem(STORAGE_KEY, payload)
  localStorage.setItem(LEGACY_STORAGE_KEY, payload)
}

export function deleteSession(id: string) {
  const sessions = loadSessions().filter((s) => s.id !== id)
  const payload = JSON.stringify(sessions)
  localStorage.setItem(STORAGE_KEY, payload)
  localStorage.setItem(LEGACY_STORAGE_KEY, payload)
}

export function loadSession(id: string): StoredSession | undefined {
  return loadSessions().find((s) => s.id === id)
}
