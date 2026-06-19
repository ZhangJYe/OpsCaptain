import { useCallback, useEffect, useMemo, useState } from 'react'
import { AlertTriangle, ArrowLeft, GitBranch, Loader2, Network, RefreshCw, Server } from 'lucide-react'
import { useTopology, type TopologyNode } from '../../hooks/useTopology'
import { cn } from '../../lib/utils'

interface Props {
  onBack: () => void
}

type Palette = {
  accent: string
  soft: string
  stroke: string
}

const CLUSTER_COLORS: Record<string, Palette> = {
  'prod-cluster-01': { accent: '#0284c7', soft: '#e0f2fe', stroke: '#7dd3fc' },
  'prod-cluster-02': { accent: '#7c3aed', soft: '#ede9fe', stroke: '#c4b5fd' },
  'staging-cluster': { accent: '#c2410c', soft: '#ffedd5', stroke: '#fdba74' },
  'dev-cluster': { accent: '#059669', soft: '#d1fae5', stroke: '#6ee7b7' },
}

const FALLBACK_PALETTES: Palette[] = [
  { accent: '#2563eb', soft: '#dbeafe', stroke: '#93c5fd' },
  { accent: '#0f766e', soft: '#ccfbf1', stroke: '#5eead4' },
  { accent: '#be123c', soft: '#ffe4e6', stroke: '#fda4af' },
  { accent: '#9333ea', soft: '#f3e8ff', stroke: '#d8b4fe' },
]

function getClusterColor(cluster?: string) {
  if (cluster && CLUSTER_COLORS[cluster]) return CLUSTER_COLORS[cluster]
  const hash = (cluster || 'default').split('').reduce((acc, c) => acc + c.charCodeAt(0), 0)
  return FALLBACK_PALETTES[hash % FALLBACK_PALETTES.length]
}

function truncate(text: string, max: number) {
  if (text.length <= max) return text
  return `${text.slice(0, Math.max(1, max - 1))}...`
}

function layoutDependencyLayers(nodes: TopologyNode[], edges: { source: string; target: string }[]) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]))
  const depsBySource = new Map<string, string[]>()
  edges.forEach((edge) => {
    if (!nodeByID.has(edge.source) || !nodeByID.has(edge.target)) return
    depsBySource.set(edge.source, [...(depsBySource.get(edge.source) || []), edge.target])
  })

  const depthByID = new Map<string, number>()
  const visiting = new Set<string>()
  const depthOf = (id: string): number => {
    if (depthByID.has(id)) return depthByID.get(id)!
    if (visiting.has(id)) return 0
    visiting.add(id)
    const deps = depsBySource.get(id) || []
    const depth = deps.length === 0 ? 0 : Math.max(...deps.map((dep) => depthOf(dep))) + 1
    visiting.delete(id)
    depthByID.set(id, depth)
    return depth
  }

  nodes.forEach((node) => depthOf(node.id))
  const layers = new Map<number, TopologyNode[]>()
  nodes.forEach((node) => {
    const depth = depthByID.get(node.id) || 0
    layers.set(depth, [...(layers.get(depth) || []), node])
  })

  const layerKeys = Array.from(layers.keys()).sort((a, b) => a - b)
  const maxLayerSize = Math.max(1, ...Array.from(layers.values()).map((items) => items.length))
  const width = Math.max(920, layerKeys.length * 250 + 160)
  const height = Math.max(460, maxLayerSize * 112 + 120)
  const positions = new Map<string, { x: number; y: number }>()

  layerKeys.forEach((depth, layerIndex) => {
    const layer = (layers.get(depth) || []).sort((a, b) => {
      const clusterCompare = (a.cluster || '').localeCompare(b.cluster || '')
      return clusterCompare || a.id.localeCompare(b.id)
    })
    const x = 96 + layerIndex * ((width - 192) / Math.max(1, layerKeys.length - 1))
    const rowGap = (height - 120) / Math.max(1, layer.length)
    layer.forEach((node, rowIndex) => {
      positions.set(node.id, {
        x,
        y: 72 + rowGap * rowIndex + rowGap / 2,
      })
    })
  })

  return { positions, width, height, layerCount: layerKeys.length }
}

function TopologyTooltip({ node, x, y }: { node: TopologyNode; x: number; y: number }) {
  return (
    <div
      className="pointer-events-none fixed z-50 min-w-44 rounded-md border border-zinc-200 bg-white px-3 py-2 text-xs shadow-xl dark:border-zinc-700 dark:bg-zinc-900"
      style={{ left: x + 14, top: y + 12 }}
    >
      <p className="font-semibold text-zinc-950 dark:text-white">{node.label || node.id}</p>
      <p className="mt-1 font-mono text-[11px] text-zinc-500 dark:text-zinc-400">{node.id}</p>
      {node.cluster && <p className="mt-1 text-zinc-600 dark:text-zinc-300">集群：{node.cluster}</p>}
      {node.owner && <p className="text-zinc-600 dark:text-zinc-300">负责人：{node.owner}</p>}
    </div>
  )
}

function EmptyState({ onRefresh }: { onRefresh: () => void }) {
  return (
    <div className="flex h-full items-center justify-center px-6">
      <div className="max-w-sm text-center">
        <Network className="mx-auto mb-3 text-zinc-400" size={28} />
        <p className="text-sm font-semibold text-zinc-900 dark:text-white">暂无拓扑数据</p>
        <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">当前筛选范围没有服务节点。</p>
        <button
          onClick={onRefresh}
          className="mt-4 inline-flex items-center gap-2 rounded-md border border-zinc-200 bg-white px-3 py-2 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800"
        >
          <RefreshCw size={14} />
          刷新拓扑
        </button>
      </div>
    </div>
  )
}

function ErrorState({ message, onRefresh }: { message: string; onRefresh: () => void }) {
  const isCMDBConfigError = message.includes('CMDB repository not configured') || message.includes('CMDB 未启用')
  return (
    <div className="flex h-full items-center justify-center px-6">
      <div className="max-w-xl rounded-lg border border-red-100 bg-red-50/80 p-5 shadow-sm dark:border-red-900/50 dark:bg-red-950/20">
        <div className="flex gap-3">
          <AlertTriangle className="mt-0.5 shrink-0 text-red-500" size={20} />
          <div>
            <p className="text-sm font-semibold text-red-900 dark:text-red-100">CMDB 拓扑不可用</p>
            <p className="mt-1 text-sm leading-6 text-red-700 dark:text-red-200">{message}</p>
            {isCMDBConfigError && (
              <div className="mt-3 space-y-1 text-xs leading-5 text-red-700/90 dark:text-red-200/90">
                <p>检查后端启动配置：`cmdb.enabled` 应为 true。</p>
                <p>检查 `cmdb.store_path` 指向的 YAML 是否可读写，容器内推荐使用 `var/runtime/cmdb/services.yaml`。</p>
                <p>修改配置或镜像后需要重启后端进程。</p>
              </div>
            )}
            <button
              onClick={onRefresh}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-red-600 px-3 py-2 text-xs font-medium text-white transition hover:bg-red-700"
            >
              <RefreshCw size={14} />
              重新加载
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function TopologyView({ onBack }: Props) {
  const { data, isLoading, error, fetchTopology } = useTopology()
  const [clusterFilter, setClusterFilter] = useState('')
  const [hoveredNode, setHoveredNode] = useState<TopologyNode | null>(null)
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 })

  useEffect(() => {
    fetchTopology(clusterFilter || undefined)
  }, [fetchTopology, clusterFilter])

  const clusters = useMemo(() => {
    if (!data?.nodes) return []
    return Array.from(new Set(data.nodes.map((node) => node.cluster).filter(Boolean) as string[])).sort()
  }, [data])

  const clusterCounts = useMemo(() => {
    const counts = new Map<string, number>()
    data?.nodes.forEach((node) => {
      const cluster = node.cluster || 'unknown'
      counts.set(cluster, (counts.get(cluster) || 0) + 1)
    })
    return Array.from(counts.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  }, [data])

  const graph = useMemo(() => {
    if (!data) return null
    return layoutDependencyLayers(data.nodes, data.edges)
  }, [data])

  const refresh = useCallback(() => {
    fetchTopology(clusterFilter || undefined)
  }, [clusterFilter, fetchTopology])

  return (
    <div className="flex h-full flex-col overflow-hidden bg-zinc-50/80 dark:bg-zinc-950">
      <div className="flex items-center gap-3 border-b border-zinc-200 bg-white px-5 py-3 dark:border-zinc-800 dark:bg-zinc-950">
        <button
          onClick={onBack}
          className="grid h-8 w-8 place-items-center rounded-md text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-800 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
          aria-label="返回"
        >
          <ArrowLeft size={18} />
        </button>
        <div className="grid h-9 w-9 place-items-center rounded-md bg-sky-50 text-sky-600 dark:bg-sky-950/40 dark:text-sky-300">
          <Network size={18} />
        </div>
        <div>
          <h2 className="text-sm font-semibold text-zinc-950 dark:text-white">服务拓扑</h2>
          <p className="text-xs text-zinc-500 dark:text-zinc-400">CMDB 依赖关系与集群分布</p>
        </div>

        <div className="ml-auto flex items-center gap-2">
          <select
            value={clusterFilter}
            onChange={(e) => setClusterFilter(e.target.value)}
            className="h-9 rounded-md border border-zinc-200 bg-white px-3 text-sm text-zinc-700 shadow-sm outline-none transition focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200"
          >
            <option value="">全部集群</option>
            {clusters.map((cluster) => (
              <option key={cluster} value={cluster}>
                {cluster}
              </option>
            ))}
          </select>
          <button
            onClick={refresh}
            disabled={isLoading}
            className="grid h-9 w-9 place-items-center rounded-md border border-zinc-200 bg-white text-zinc-500 shadow-sm transition hover:bg-zinc-50 hover:text-zinc-800 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
            aria-label="刷新拓扑"
          >
            <RefreshCw size={16} className={cn(isLoading && 'animate-spin')} />
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(0,1fr)_280px]">
        <main className="relative min-h-0 overflow-auto">
          {isLoading && !data && (
            <div className="flex h-full items-center justify-center">
              <Loader2 size={26} className="animate-spin text-sky-500" />
            </div>
          )}

          {error && <ErrorState message={error} onRefresh={refresh} />}

          {!isLoading && !error && data && data.nodes.length === 0 && <EmptyState onRefresh={refresh} />}

          {data && data.nodes.length > 0 && graph && !error && (
            <div className="min-h-full p-5">
              <div className="mb-4 grid grid-cols-3 gap-3">
                <div className="rounded-md border border-zinc-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900">
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">服务节点</p>
                  <p className="mt-1 text-2xl font-semibold text-zinc-950 dark:text-white">{data.nodes.length}</p>
                </div>
                <div className="rounded-md border border-zinc-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900">
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">依赖关系</p>
                  <p className="mt-1 text-2xl font-semibold text-zinc-950 dark:text-white">{data.edges.length}</p>
                </div>
                <div className="rounded-md border border-zinc-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900">
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">依赖层级</p>
                  <p className="mt-1 text-2xl font-semibold text-zinc-950 dark:text-white">{graph.layerCount}</p>
                </div>
              </div>

              <div className="relative overflow-auto rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-950">
                <div className="absolute inset-0 opacity-70 [background-image:linear-gradient(to_right,rgba(148,163,184,.14)_1px,transparent_1px),linear-gradient(to_bottom,rgba(148,163,184,.14)_1px,transparent_1px)] [background-size:32px_32px]" />
                <svg
                  viewBox={`0 0 ${graph.width} ${graph.height}`}
                  className="relative z-10 min-h-[520px] w-full"
                  role="img"
                  aria-label="服务依赖拓扑图"
                >
                  <defs>
                    <marker id="topology-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
                      <path d="M 0 0 L 10 5 L 0 10 z" fill="#94a3b8" />
                    </marker>
                  </defs>

                  {data.edges.map((edge, index) => {
                    const from = graph.positions.get(edge.source)
                    const to = graph.positions.get(edge.target)
                    if (!from || !to) return null
                    const startX = from.x - 78
                    const startY = from.y
                    const endX = to.x + 78
                    const endY = to.y
                    const midX = (startX + endX) / 2
                    return (
                      <path
                        key={`${edge.source}-${edge.target}-${index}`}
                        d={`M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}`}
                        fill="none"
                        stroke="#cbd5e1"
                        strokeWidth={1.4}
                        markerEnd="url(#topology-arrow)"
                      />
                    )
                  })}

                  {data.nodes.map((node) => {
                    const pos = graph.positions.get(node.id)
                    if (!pos) return null
                    const palette = getClusterColor(node.cluster)
                    const active = hoveredNode?.id === node.id
                    return (
                      <g
                        key={node.id}
                        transform={`translate(${pos.x - 78}, ${pos.y - 29})`}
                        onMouseEnter={(event) => {
                          setHoveredNode(node)
                          setTooltipPos({ x: event.clientX, y: event.clientY })
                        }}
                        onMouseMove={(event) => setTooltipPos({ x: event.clientX, y: event.clientY })}
                        onMouseLeave={() => setHoveredNode(null)}
                        className="cursor-pointer"
                      >
                        <rect
                          width="156"
                          height="58"
                          rx="12"
                          fill={active ? palette.soft : '#ffffff'}
                          stroke={active ? palette.accent : palette.stroke}
                          strokeWidth={active ? 2 : 1.3}
                          filter={active ? 'drop-shadow(0 12px 18px rgba(15, 23, 42, 0.14))' : undefined}
                        />
                        <circle cx="19" cy="20" r="6" fill={palette.accent} />
                        <text x="33" y="23" fill="#18181b" fontSize="13" fontWeight="700">
                          {truncate(node.label || node.id, 13)}
                        </text>
                        <text x="18" y="43" fill="#71717a" fontSize="10" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">
                          {truncate(node.id, 18)}
                        </text>
                      </g>
                    )
                  })}
                </svg>
              </div>
            </div>
          )}
        </main>

        <aside className="border-t border-zinc-200 bg-white p-5 dark:border-zinc-800 dark:bg-zinc-950 lg:border-l lg:border-t-0">
          <div className="mb-6">
            <p className="text-xs font-medium uppercase tracking-wide text-zinc-400">当前范围</p>
            <p className="mt-2 text-sm font-semibold text-zinc-950 dark:text-white">{clusterFilter || '全部集群'}</p>
          </div>

          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-zinc-900 dark:text-white">
              <Server size={16} className="text-zinc-400" />
              集群分布
            </div>
            {clusterCounts.length === 0 ? (
              <p className="text-xs text-zinc-500 dark:text-zinc-400">暂无服务节点。</p>
            ) : (
              <div className="space-y-2">
                {clusterCounts.map(([cluster, count]) => {
                  const palette = getClusterColor(cluster)
                  return (
                    <div key={cluster} className="flex items-center justify-between gap-3 rounded-md border border-zinc-200 px-3 py-2 dark:border-zinc-800">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: palette.accent }} />
                        <span className="truncate text-xs text-zinc-700 dark:text-zinc-300">{cluster}</span>
                      </div>
                      <span className="text-xs font-semibold text-zinc-900 dark:text-white">{count}</span>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          <div className="mt-7 border-t border-zinc-200 pt-5 dark:border-zinc-800">
            <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-zinc-900 dark:text-white">
              <GitBranch size={16} className="text-zinc-400" />
              读图口径
            </div>
            <div className="space-y-2 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
              <p>箭头表示 depends_on，箭头指向被依赖服务。</p>
              <p>越靠左越接近基础依赖，越靠右越接近入口或调用方。</p>
              <p>悬停节点可查看服务 ID、集群和负责人。</p>
            </div>
          </div>
        </aside>
      </div>

      {hoveredNode && <TopologyTooltip node={hoveredNode} x={tooltipPos.x} y={tooltipPos.y} />}
    </div>
  )
}
