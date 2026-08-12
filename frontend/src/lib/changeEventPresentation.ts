import type { ChangeEventItem, ChangeEventRisk, ChangeEventStreamStatus } from '../hooks/useChangeEvents'

const RISK_LABELS: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
  critical: '严重',
}

const TYPE_LABELS: Record<string, string> = {
  deploy: '发布',
  rollback: '回滚',
  git_push: '代码推送',
  pipeline: '流水线',
  config_update: '配置',
  scale: '扩缩容',
  restart: '重启',
  resource_update: '资源变更',
  dns_switch: '流量切换',
  failover: '故障切换',
  maintenance: '维护',
}

export function changeEventRiskLabel(risk: ChangeEventRisk): string {
  const normalized = String(risk || 'low').toLowerCase()
  return RISK_LABELS[normalized] || normalized
}

export function changeEventTypeLabel(type: string): string {
  return TYPE_LABELS[type] || type.replace(/_/g, ' ')
}

export function changeEventRiskTone(risk: ChangeEventRisk): string {
  switch (String(risk || 'low').toLowerCase()) {
    case 'critical':
      return 'border-rose-300 bg-rose-50 text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-300'
    case 'high':
      return 'border-orange-300 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300'
    case 'medium':
      return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
    default:
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
  }
}

export function changeEventStreamLabel(status: ChangeEventStreamStatus): string {
  switch (status) {
    case 'open':
      return '监听中'
    case 'connecting':
      return '连接中'
    case 'error':
      return '未连接'
    default:
      return '已关闭'
  }
}

export function changeEventStreamTone(status: ChangeEventStreamStatus): string {
  return status === 'open'
    ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.65)]'
    : status === 'connecting'
      ? 'bg-sky-400 shadow-[0_0_10px_rgba(56,189,248,0.65)] animate-pulse'
      : status === 'error'
        ? 'bg-rose-400 shadow-[0_0_10px_rgba(251,113,133,0.65)]'
        : 'bg-zinc-300 dark:bg-zinc-600'
}

export interface PetSentinelVisualState {
  ringClass: string
  statusClass: string
  statusLabel: string
}

export function petSentinelVisualState(
  status: ChangeEventStreamStatus,
  latestEvent?: ChangeEventItem,
  unreadCount = 0,
): PetSentinelVisualState {
  const risk = String(latestEvent?.risk_level || 'low').toLowerCase()
  const riskRing = risk === 'critical'
    ? 'border-rose-300/80 dark:border-rose-400/50'
    : risk === 'high'
      ? 'border-orange-300/80 dark:border-orange-400/50'
      : risk === 'medium'
        ? 'border-amber-300/80 dark:border-amber-400/50'
        : 'border-emerald-300/70 dark:border-emerald-400/45'

  if (status === 'error') {
    return { ringClass: 'border-rose-300/80 dark:border-rose-400/50', statusClass: changeEventStreamTone(status), statusLabel: changeEventStreamLabel(status) }
  }
  if (status === 'connecting') {
    return { ringClass: 'border-sky-300/80 dark:border-sky-400/50', statusClass: changeEventStreamTone(status), statusLabel: changeEventStreamLabel(status) }
  }
  return {
    ringClass: unreadCount > 0 ? riskRing : 'border-sky-300/50 dark:border-sky-400/35',
    statusClass: changeEventStreamTone(status),
    statusLabel: changeEventStreamLabel(status),
  }
}

export function isPetClick(start: { x: number; y: number }, end: { x: number; y: number }, threshold = 6): boolean {
  return Math.hypot(end.x - start.x, end.y - start.y) < threshold
}
