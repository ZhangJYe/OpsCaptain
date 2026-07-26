import { useCallback, useEffect, useRef, useState } from 'react'
import type { AIOpsEngine, IncidentEvent, IncidentSession, IncidentTurn } from '../types/chat'
import { getApiBaseUrl } from '../lib/utils'

function unwrapPayload(data: any): any {
  if (data && typeof data === 'object' && data.data) {
    return data.data
  }
  return data
}

async function readPayload(res: Response): Promise<any> {
  const raw = await res.text()
  if (!raw.trim()) {
    return {}
  }
  try {
    return JSON.parse(raw)
  } catch {
    return {}
  }
}

function latestTurn(incident: IncidentSession | null): IncidentTurn | undefined {
  return incident?.turns?.[incident.turns.length - 1]
}

function normalizeIncident(value: any): IncidentSession {
  return {
    incident_id: String(value?.incident_id || ''),
    session_id: String(value?.session_id || ''),
    title: String(value?.title || '未命名事故'),
    status: value?.status || 'active',
    engine_strategy: String(value?.engine_strategy || 'plan_execute_replan'),
    latest_summary: String(value?.latest_summary || ''),
    turns: Array.isArray(value?.turns) ? value.turns : [],
    events: Array.isArray(value?.events) ? value.events : [],
    created_at: Number(value?.created_at || Date.now()),
    updated_at: Number(value?.updated_at || Date.now()),
  }
}

async function readIncidentResponse(res: Response): Promise<IncidentSession> {
  const data = await readPayload(res)
  const payload = unwrapPayload(data)
  if (!res.ok) {
    throw new Error(String(data?.message || payload?.message || `HTTP ${res.status}`))
  }
  return normalizeIncident(payload?.incident)
}

function mergeIncidentEvent(incident: IncidentSession | null, event: IncidentEvent): IncidentSession | null {
  if (!incident || incident.incident_id !== event.incident_id) {
    return incident
  }
  if (incident.events.some((item) => item.event_id === event.event_id)) {
    return incident
  }
  return {
    ...incident,
    events: [...incident.events, event],
    updated_at: Math.max(incident.updated_at || 0, event.created_at || 0),
  }
}

export function useIncidents() {
  const [incident, setIncident] = useState<IncidentSession | null>(null)
  const [incidents, setIncidents] = useState<IncidentSession[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  const closeEvents = useCallback(() => {
    eventSourceRef.current?.close()
    eventSourceRef.current = null
  }, [])

  const refreshList = useCallback(async () => {
    const res = await fetch(`${getApiBaseUrl()}/ai_ops/incidents`)
    const data = await readPayload(res)
    const payload = unwrapPayload(data)
    if (!res.ok) {
      throw new Error(String(data?.message || payload?.message || `HTTP ${res.status}`))
    }
    const items = Array.isArray(payload?.items) ? payload.items.map(normalizeIncident) : []
    setIncidents(items)
    return items
  }, [])

  const refreshIncident = useCallback(async (incidentId: string) => {
    const res = await fetch(`${getApiBaseUrl()}/ai_ops/incidents/${encodeURIComponent(incidentId)}`)
    const next = await readIncidentResponse(res)
    setIncident(next)
    return next
  }, [])

  const subscribe = useCallback(
    (incidentId: string, turnId?: string) => {
      closeEvents()
      const suffix = turnId ? `?turn_id=${encodeURIComponent(turnId)}` : ''
      const source = new EventSource(
        `${getApiBaseUrl()}/ai_ops/incidents/${encodeURIComponent(incidentId)}/events${suffix}`,
      )
      eventSourceRef.current = source
      setIsLoading(true)
      source.addEventListener('incident_event', (raw) => {
        try {
          const event = JSON.parse((raw as MessageEvent).data) as IncidentEvent
          setIncident((current) => mergeIncidentEvent(current, event))
        } catch {
          return
        }
      })
      source.addEventListener('done', () => {
        closeEvents()
        setIsLoading(false)
        void refreshIncident(incidentId)
        void refreshList()
      })
      source.addEventListener('error', () => {
        setError('排障事件流已中断，正在刷新事故状态。')
        closeEvents()
        setIsLoading(false)
        void refreshIncident(incidentId)
      })
    },
    [closeEvents, refreshIncident, refreshList],
  )

  const loadIncident = useCallback(
    async (incidentId: string) => {
      setError(null)
      closeEvents()
      setIsLoading(true)
      try {
        const next = await refreshIncident(incidentId)
        const turn = latestTurn(next)
        if (next.status === 'running') {
          subscribe(next.incident_id, turn?.turn_id)
        } else {
          setIsLoading(false)
        }
        return next
      } catch (err: any) {
        setIsLoading(false)
        setError(err?.message || '事故记录加载失败。')
        throw err
      }
    },
    [closeEvents, refreshIncident, subscribe],
  )

  const createIncident = useCallback(
    async (query: string, engine: AIOpsEngine, selectedSkillIds?: string[]) => {
      setError(null)
      closeEvents()
      setIsLoading(true)
      try {
        const res = await fetch(`${getApiBaseUrl()}/ai_ops/incidents`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, engine, selected_skill_ids: selectedSkillIds }),
        })
        const next = await readIncidentResponse(res)
        setIncident(next)
        void refreshList()
        subscribe(next.incident_id, latestTurn(next)?.turn_id)
        return next
      } catch (err: any) {
        setIsLoading(false)
        setError(err?.message || '事故创建失败。')
        throw err
      }
    },
    [closeEvents, refreshList, subscribe],
  )

  const appendTurn = useCallback(
    async (query: string, selectedSkillIds?: string[]) => {
      if (!incident) {
        throw new Error('请先创建事故。')
      }
      setError(null)
      closeEvents()
      setIsLoading(true)
      try {
        const res = await fetch(
          `${getApiBaseUrl()}/ai_ops/incidents/${encodeURIComponent(incident.incident_id)}/turns`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ query, selected_skill_ids: selectedSkillIds }),
          },
        )
        const next = await readIncidentResponse(res)
        setIncident(next)
        void refreshList()
        subscribe(next.incident_id, latestTurn(next)?.turn_id)
        return next
      } catch (err: any) {
        setIsLoading(false)
        setError(err?.message || '追加排障轮次失败。')
        throw err
      }
    },
    [closeEvents, incident, refreshList, subscribe],
  )

  const newIncident = useCallback(() => {
    closeEvents()
    setIncident(null)
    setIsLoading(false)
    setError(null)
  }, [closeEvents])

  useEffect(() => {
    void refreshList().catch((err: any) => {
      setError(err?.message || '事故记录加载失败。')
    })
    return closeEvents
  }, [closeEvents, refreshList])

  return {
    incident,
    incidents,
    isLoading,
    error,
    createIncident,
    appendTurn,
    loadIncident,
    newIncident,
    refreshList,
  }
}
