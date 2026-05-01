import { useState, useEffect, useCallback } from 'react'
import { Server, Radio, Eye } from 'lucide-react'
import type { EndpointStatus } from '../../types/chat'

interface Props {}

const observabilityTargets = [
  { name: 'Backend', probeUrl: '/ai/readyz', link: '/ai/readyz' },
  { name: 'Jaeger', probeUrl: '/ai/jaeger/', link: '/ai/jaeger/' },
  { name: 'Prometheus', probeUrl: '/ai/prometheus/-/healthy', link: '/ai/prometheus/' },
] as const

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
      observabilityTargets.map(async ({ name, probeUrl, link }) => {
        try {
          const res = await fetch(probeUrl, { signal: AbortSignal.timeout(5000) })
          const status = res.ok ? ('healthy' as const) : ('degraded' as const)
          return { name, status, text: res.ok ? '正常' : `${res.status}`, link, lastCheck: now }
        } catch {
          return { name, status: 'down' as const, text: '不可达', link, lastCheck: now }
        }
      })
    )

    const newEndpoints = results.map((r) =>
      r.status === 'fulfilled' ? r.value : { name: '', status: 'down' as const, text: '不可达', link: '', lastCheck: now }
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
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs text-zinc-600 dark:text-zinc-500">服务状态</p>
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
            <div
              key={ep.name}
              className="flex items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-800/30"
            >
              <Icon size={14} className="text-zinc-500 dark:text-zinc-500" />
              <span className="text-xs flex-1">{ep.name}</span>
              <span className={`inline-block w-1.5 h-1.5 rounded-full ${statusColors[ep.status]}`} />
              <span className="w-10 text-right text-[10px] text-zinc-500 dark:text-zinc-600">{ep.text}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
