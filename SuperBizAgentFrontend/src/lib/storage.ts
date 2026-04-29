import type { ChatMessage } from '../types/chat'

const STORAGE_KEY = 'opscaption-chat-history'
const MAX_HISTORY = 50

interface StoredSession {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
}

export function loadSessions(): StoredSession[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

export function saveSession(id: string, messages: ChatMessage[]) {
  const sessions = loadSessions()
  const idx = sessions.findIndex((s) => s.id === id)
  const title = messages[0]?.content?.slice(0, 50) || '新会话'
  const session: StoredSession = {
    id,
    title,
    messages: messages.slice(-50),
    createdAt: idx >= 0 ? sessions[idx].createdAt : Date.now(),
    updatedAt: Date.now(),
  }
  if (idx >= 0) {
    sessions[idx] = session
  } else {
    sessions.unshift(session)
  }
  if (sessions.length > MAX_HISTORY) {
    sessions.length = MAX_HISTORY
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(sessions))
}

export function deleteSession(id: string) {
  const sessions = loadSessions().filter((s) => s.id !== id)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(sessions))
}

export function loadSession(id: string): StoredSession | undefined {
  return loadSessions().find((s) => s.id === id)
}
