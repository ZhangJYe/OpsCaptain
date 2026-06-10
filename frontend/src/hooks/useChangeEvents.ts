import { useCallback, useEffect, useRef, useState } from 'react'
import { getAccessToken, getApiBaseUrl } from '../lib/utils'

export type ChangeEventRisk = 'low' | 'medium' | 'high' | 'critical' | string

export interface ChangeEventItem {
  event_id: string
  source: string
  event_type: string
  service: string
  env: string
  namespace?: string
  cluster?: string
  summary: string
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  diff?: string
  risk_level: ChangeEventRisk
  operator?: string
  started_at: string
  finished_at?: string
  correlation_keys?: string[]
}

export type ChangeEventStreamStatus = 'connecting' | 'open' | 'error' | 'closed'

function normalizeChangeEvent(raw: any): ChangeEventItem | null {
  if (!raw || typeof raw !== 'object') {
    return null
  }
  const eventId = String(raw.event_id || '').trim()
  const service = String(raw.service || '').trim()
  const eventType = String(raw.event_type || '').trim()
  if (!eventId || !service || !eventType) {
    return null
  }
  return {
    event_id: eventId,
    source: String(raw.source || 'unknown'),
    event_type: eventType,
    service,
    env: String(raw.env || 'unknown'),
    namespace: raw.namespace ? String(raw.namespace) : undefined,
    cluster: raw.cluster ? String(raw.cluster) : undefined,
    summary: String(raw.summary || `${service} ${eventType}`),
    before: raw.before && typeof raw.before === 'object' ? raw.before : undefined,
    after: raw.after && typeof raw.after === 'object' ? raw.after : undefined,
    diff: raw.diff ? String(raw.diff) : undefined,
    risk_level: String(raw.risk_level || 'low'),
    operator: raw.operator ? String(raw.operator) : undefined,
    started_at: String(raw.started_at || new Date().toISOString()),
    finished_at: raw.finished_at ? String(raw.finished_at) : undefined,
    correlation_keys: Array.isArray(raw.correlation_keys) ? raw.correlation_keys.map(String) : undefined,
  }
}

export function useChangeEvents() {
  const [events, setEvents] = useState<ChangeEventItem[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [status, setStatus] = useState<ChangeEventStreamStatus>('connecting')
  const sourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (typeof window === 'undefined') {
      return undefined
    }

    const token = getAccessToken()
    // SSE 端点需要鉴权（后端 RequiredRolesForPath: Operator/Admin）。
    // EventSource 无法发送 header，token 通过查询参数传递。
    const url = token
      ? `${getApiBaseUrl()}/change_events/stream?access_token=${encodeURIComponent(token)}`
      : `${getApiBaseUrl()}/change_events/stream`
    const source = new EventSource(url)
    sourceRef.current = source
    setStatus('connecting')

    source.onopen = () => {
      setStatus('open')
    }

    source.addEventListener('heartbeat', () => {
      setStatus('open')
    })

    source.addEventListener('change_event', (message) => {
      try {
        const parsed = normalizeChangeEvent(JSON.parse((message as MessageEvent).data))
        if (!parsed) {
          return
        }
        setEvents((current) => {
          const next = [parsed, ...current.filter((item) => item.event_id !== parsed.event_id)]
          return next.slice(0, 20)
        })
        setUnreadCount((count) => Math.min(count + 1, 99))
        setStatus('open')
      } catch {
        return
      }
    })

    source.onerror = () => {
      setStatus('error')
    }

    return () => {
      source.close()
      sourceRef.current = null
      setStatus('closed')
    }
  }, [])

  const markRead = useCallback(() => {
    setUnreadCount(0)
  }, [])

  const clear = useCallback(() => {
    setEvents([])
    setUnreadCount(0)
  }, [])

  return {
    events,
    latestEvent: events[0],
    unreadCount,
    status,
    markRead,
    clear,
  }
}
