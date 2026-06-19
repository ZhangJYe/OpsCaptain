import { useCallback, useState } from 'react'
import { getApiBaseUrl } from '../lib/utils'

export interface TopologyNode {
  id: string
  label: string
  type: string
  cluster?: string
  owner?: string
}

export interface TopologyEdge {
  source: string
  target: string
  type: string
}

export interface TopologyData {
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

export function useTopology() {
  const [data, setData] = useState<TopologyData | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchTopology = useCallback(async (cluster?: string, service?: string) => {
    setIsLoading(true)
    setError(null)
    try {
      const params = new URLSearchParams()
      if (cluster) params.set('cluster', cluster)
      if (service) params.set('service', service)
      const qs = params.toString()
      const url = `${getApiBaseUrl()}/cmdb/topology${qs ? `?${qs}` : ''}`
      const res = await fetch(url)
      const raw = await res.text()
      if (!res.ok) {
        throw new Error(`后端拓扑接口返回 ${res.status}，请确认 Backend 已启动且 /api 代理指向正确。`)
      }
      if (!raw) {
        throw new Error('后端拓扑接口返回空响应，请检查 Backend 启动状态。')
      }
      const payload = raw ? JSON.parse(raw) : {}
      const unwrapped = payload?.data ?? payload
      if (!unwrapped?.success) {
        throw new Error(unwrapped?.error || '后端拓扑接口未返回可用数据。')
      }
      setData({
        nodes: Array.isArray(unwrapped.nodes) ? unwrapped.nodes : [],
        edges: Array.isArray(unwrapped.edges) ? unwrapped.edges : [],
      })
    } catch (e: any) {
      const message = e?.message || ''
      if (message === 'Failed to fetch') {
        setError('后端拓扑接口不可达，请确认 Backend 已启动且 /api 代理指向正确。')
      } else {
        setError(message || 'Failed to fetch topology')
      }
    } finally {
      setIsLoading(false)
    }
  }, [])

  return { data, isLoading, error, fetchTopology }
}
