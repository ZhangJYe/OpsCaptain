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
    <div className="rounded-xl border border-white/60 bg-white/55 p-3 backdrop-blur-md dark:border-white/10 dark:bg-slate-800/40">
      <div className="mb-2.5 flex items-center justify-between">
        <p className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">服务状态</p>
        <button
          onClick={probe}
          className="rounded-md px-1.5 py-0.5 text-[10px] font-medium text-sky-600 transition-colors hover:bg-sky-500/10 dark:text-sky-400"
        >
          刷新
        </button>
      </div>
      <div className="space-y-1">
        {endpoints.map((ep) => {
          const Icon = icons[ep.name] || Server
          return (
            <a
              key={ep.name}
              href={ep.link}
              target="_blank"
              rel="noreferrer"
              className="grid grid-cols-[18px_minmax(0,1fr)_8px_44px] items-center gap-2 rounded-lg px-2 py-2 transition-colors hover:bg-white/70 dark:hover:bg-slate-700/40"
            >
              <Icon size={14} className="text-zinc-500 dark:text-zinc-500" />
              <span className="truncate text-xs font-medium text-zinc-700 dark:text-zinc-300">{ep.name}</span>
              <span className={`inline-block w-1.5 h-1.5 rounded-full ${statusColors[ep.status]}`} />
              <span className="text-right text-[10px] font-medium text-zinc-500 dark:text-zinc-500">{ep.text}</span>
            </a>
          )
        })}
      </div>
    </div>
  )
}
