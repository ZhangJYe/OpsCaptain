import { useState, useEffect, useCallback } from 'react'
import { Server, Radio, Eye } from 'lucide-react'
import type { EndpointStatus } from '../../types/chat'

interface Props {}

export function ObservabilityPanel({}: Props) {
  const [endpoints, setEndpoints] = useState<EndpointStatus[]>([
    { name: 'Backend', status: 'checking', text: '检测中...', link: '/ai/readyz', lastCheck: 0 },
    { name: 'Jaeger', status: 'checking', text: '检测中...', link: '/ai/jaeger/', lastCheck: 0 },
    { name: 'Prometheus', status: 'checking', text: '检测中...', link: '/ai/prometheus/', lastCheck: 0 },
  ])

  const probe = useCallback(async () => {
    const now = Date.now()
    setEndpoints((prev) =>
      prev.map((ep) => ({ ...ep, status: 'checking' as const, text: '检测中...' }))
    )

    const results = await Promise.allSettled(
      [
        { name: 'Backend', url: '/ai/readyz' },
        { name: 'Jaeger', url: '/ai/jaeger/' },
        { name: 'Prometheus', url: '/ai/prometheus/-/healthy' },
      ].map(async ({ name, url }) => {
        try {
          const res = await fetch(url, { signal: AbortSignal.timeout(5000) })
          const status = res.ok ? ('healthy' as const) : ('degraded' as const)
          return { name, status, text: res.ok ? '正常' : `${res.status}`, lastCheck: now }
        } catch {
          return { name, status: 'down' as const, text: '不可达', lastCheck: now }
        }
      })
    )

    const newEndpoints = results.map((r) =>
      r.status === 'fulfilled' ? r.value : { name: '', status: 'down' as const, text: '不可达', lastCheck: now }
    )
    setEndpoints((prev) =>
      prev.map((ep) => newEndpoints.find((n) => n.name === ep.name) || ep)
    )
  }, [])

  useEffect(() => {
    probe()
    const timer = setInterval(probe, 60000)
    return () => clearInterval(timer)
  }, [probe])

  const statusColors: Record<string, string> = {
    healthy: 'bg-emerald-500',
    degraded: 'bg-amber-500',
    down: 'bg-red-500',
    checking: 'bg-zinc-600 animate-pulse',
  }

  const icons: Record<string, typeof Server> = {
    Backend: Server,
    Jaeger: Eye,
    Prometheus: Radio,
  }

  return (
    <div className="glass rounded-xl p-3">
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs text-zinc-500">服务状态</p>
        <button
          onClick={probe}
          className="text-[10px] text-accent hover:underline"
        >
          刷新
        </button>
      </div>
      <div className="space-y-1.5">
        {endpoints.map((ep) => {
          const Icon = icons[ep.name] || Server
          return (
            <div key={ep.name} className="flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-zinc-800/30 transition-colors">
              <Icon size={14} className="text-zinc-500" />
              <span className="text-xs flex-1">{ep.name}</span>
              <span className={`inline-block w-1.5 h-1.5 rounded-full ${statusColors[ep.status]}`} />
              <span className="text-[10px] text-zinc-600 w-10 text-right">{ep.text}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
