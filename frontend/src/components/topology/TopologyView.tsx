import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, Loader2, Network, RefreshCw } from 'lucide-react'
import { useTopology, type TopologyData, type TopologyEdge, type TopologyNode } from '../../hooks/useTopology'

interface Props {
  onBack: () => void
}

const CLUSTER_COLORS: Record<string, { fill: string; stroke: string; text: string }> = {
  'prod-cluster-01': { fill: '#0ea5e9', stroke: '#0284c7', text: '#0c4a6e' },
  'prod-cluster-02': { fill: '#8b5cf6', stroke: '#7c3aed', text: '#4c1d95' },
  'staging-cluster': { fill: '#f59e0b', stroke: '#d97706', text: '#78350f' },
  'dev-cluster': { fill: '#10b981', stroke: '#059669', text: '#064e3b' },
}

function getClusterColor(cluster?: string) {
  if (cluster && CLUSTER_COLORS[cluster]) return CLUSTER_COLORS[cluster]
  const fallbackColors = ['#6366f1', '#ec4899', '#14b8a6', '#f97316', '#84cc16']
  const hash = (cluster || 'default').split('').reduce((acc, c) => acc + c.charCodeAt(0), 0)
  const color = fallbackColors[hash % fallbackColors.length]
  return { fill: color, stroke: color, text: '#fff' }
}

function layoutGrid(nodes: TopologyNode[], width: number, height: number): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  const cols = Math.ceil(Math.sqrt(nodes.length))
  const rows = Math.ceil(nodes.length / cols)
  const cellW = width / (cols + 1)
  const cellH = height / (rows + 1)

  nodes.forEach((node, i) => {
    const col = i % cols
    const row = Math.floor(i / cols)
    pos.set(node.id, {
      x: cellW * (col + 1),
      y: cellH * (row + 1),
    })
  })
  return pos
}

function SvgArrowMarker() {
  return (
    <defs>
      <marker
        id="arrowhead"
        viewBox="0 0 10 7"
        refX="10"
        refY="3.5"
        markerWidth="8"
        markerHeight="6"
        orient="auto-start-reverse"
      >
        <polygon points="0 0, 10 3.5, 0 7" fill="#94a3b8" />
      </marker>
    </defs>
  )
}

function TopologyTooltip({ node, x, y }: { node: TopologyNode; x: number; y: number }) {
  return (
    <div
      className="pointer-events-none fixed z-50 rounded-lg border border-white/60 bg-white/90 px-3 py-2 text-xs shadow-xl backdrop-blur-2xl dark:border-zinc-700/60 dark:bg-zinc-900/90"
      style={{ left: x + 12, top: y - 8 }}
    >
      <p className="font-semibold text-zinc-900 dark:text-white">{node.label || node.id}</p>
      {node.cluster && (
        <p className="mt-0.5 text-zinc-500 dark:text-zinc-400">集群: {node.cluster}</p>
      )}
      {node.owner && (
        <p className="text-zinc-500 dark:text-zinc-400">负责人: {node.owner}</p>
      )}
    </div>
  )
}

export function TopologyView({ onBack }: Props) {
  const { data, isLoading, error, fetchTopology } = useTopology()
  const [clusterFilter, setClusterFilter] = useState('')
  const [hoveredNode, setHoveredNode] = useState<TopologyNode | null>(null)
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 })
  const svgRef = useRef<SVGSVGElement>(null)

  const svgWidth = 800
  const svgHeight = 600

  useEffect(() => {
    fetchTopology(clusterFilter || undefined)
  }, [fetchTopology, clusterFilter])

  const clusters = useMemo(() => {
    if (!data?.nodes) return []
    const set = new Set<string>()
    data.nodes.forEach((n) => {
      if (n.cluster) set.add(n.cluster)
    })
    return Array.from(set).sort()
  }, [data])

  const filteredData = useMemo<TopologyData | null>(() => {
    if (!data) return null
    if (!clusterFilter) return data
    const nodeIds = new Set(data.nodes.filter((n) => n.cluster === clusterFilter).map((n) => n.id))
    const filteredNodes = data.nodes.filter((n) => nodeIds.has(n.id))
    const filteredEdges = data.edges.filter((e) => nodeIds.has(e.source) || nodeIds.has(e.target))
    return { nodes: filteredNodes, edges: filteredEdges }
  }, [data, clusterFilter])

  const positions = useMemo(() => {
    if (!filteredData?.nodes) return new Map()
    return layoutGrid(filteredData.nodes, svgWidth, svgHeight)
  }, [filteredData])

  const handleNodeHover = useCallback((node: TopologyNode, e: React.MouseEvent) => {
    setHoveredNode(node)
    setTooltipPos({ x: e.clientX, y: e.clientY })
  }, [])

  const handleNodeLeave = useCallback(() => {
    setHoveredNode(null)
  }, [])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex items-center gap-3 border-b border-zinc-200/80 bg-white/80 px-4 py-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
        <button
          onClick={onBack}
          className="rounded-lg p-1.5 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
        >
          <ArrowLeft size={18} />
        </button>
        <Network size={18} className="text-sky-500" />
        <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">服务拓扑</h2>

        <div className="ml-auto flex items-center gap-2">
          <select
            value={clusterFilter}
            onChange={(e) => setClusterFilter(e.target.value)}
            className="rounded-lg border border-zinc-200 bg-white px-2.5 py-1.5 text-xs text-zinc-700 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-300"
          >
            <option value="">全部集群</option>
            {clusters.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          <button
            onClick={() => fetchTopology(clusterFilter || undefined)}
            disabled={isLoading}
            className="rounded-lg p-1.5 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
          >
            <RefreshCw size={16} className={isLoading ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-auto p-4">
        {isLoading && !data && (
          <div className="flex h-full items-center justify-center">
            <Loader2 size={24} className="animate-spin text-sky-500" />
          </div>
        )}

        {error && (
          <div className="flex h-full items-center justify-center">
            <p className="text-sm text-red-500">{error}</p>
          </div>
        )}

        {!isLoading && !error && filteredData && filteredData.nodes.length === 0 && (
          <div className="flex h-full items-center justify-center">
            <p className="text-sm text-zinc-500">暂无拓扑数据</p>
          </div>
        )}

        {filteredData && filteredData.nodes.length > 0 && (
          <div className="rounded-xl border border-white/60 bg-white/70 p-4 backdrop-blur-2xl dark:border-zinc-800/60 dark:bg-zinc-900/60">
            <div className="mb-3 flex items-center gap-4 text-xs text-zinc-500 dark:text-zinc-400">
              <span>{filteredData.nodes.length} 个服务</span>
              <span>{filteredData.edges.length} 条依赖</span>
              {clusterFilter && <span>集群: {clusterFilter}</span>}
            </div>

            <svg
              ref={svgRef}
              viewBox={`0 0 ${svgWidth} ${svgHeight}`}
              className="h-auto w-full"
              style={{ minHeight: 400 }}
            >
              <SvgArrowMarker />

              {filteredData.edges.map((edge, i) => {
                const from = positions.get(edge.source)
                const to = positions.get(edge.target)
                if (!from || !to) return null
                const dx = to.x - from.x
                const dy = to.y - from.y
                const len = Math.sqrt(dx * dx + dy * dy)
                if (len === 0) return null
                const nodeR = 32
                const ux = dx / len
                const uy = dy / len
                const x1 = from.x + ux * nodeR
                const y1 = from.y + uy * nodeR
                const x2 = to.x - ux * nodeR
                const y2 = to.y - uy * nodeR
                return (
                  <line
                    key={`edge-${i}`}
                    x1={x1}
                    y1={y1}
                    x2={x2}
                    y2={y2}
                    stroke="#cbd5e1"
                    strokeWidth={1.5}
                    markerEnd="url(#arrowhead)"
                    className="dark:stroke-zinc-700"
                  />
                )
              })}

              {filteredData.nodes.map((node) => {
                const pos = positions.get(node.id)
                if (!pos) return null
                const colors = getClusterColor(node.cluster)
                return (
                  <g
                    key={node.id}
                    onMouseEnter={(e) => handleNodeHover(node, e)}
                    onMouseMove={(e) => setTooltipPos({ x: e.clientX, y: e.clientY })}
                    onMouseLeave={handleNodeLeave}
                    className="cursor-pointer"
                  >
                    <circle
                      cx={pos.x}
                      cy={pos.y}
                      r={30}
                      fill={colors.fill}
                      stroke={colors.stroke}
                      strokeWidth={2}
                      opacity={0.9}
                    />
                    <text
                      x={pos.x}
                      y={pos.y - 4}
                      textAnchor="middle"
                      fill={colors.text}
                      fontSize={10}
                      fontWeight={600}
                    >
                      {(node.label || node.id).length > 8
                        ? (node.label || node.id).slice(0, 8) + '…'
                        : node.label || node.id}
                    </text>
                    <text
                      x={pos.x}
                      y={pos.y + 10}
                      textAnchor="middle"
                      fill={colors.text}
                      fontSize={8}
                      opacity={0.7}
                    >
                      {node.id.length > 10 ? node.id.slice(0, 10) + '…' : node.id}
                    </text>
                  </g>
                )
              })}
            </svg>
          </div>
        )}
      </div>

      {hoveredNode && <TopologyTooltip node={hoveredNode} x={tooltipPos.x} y={tooltipPos.y} />}
    </div>
  )
}
