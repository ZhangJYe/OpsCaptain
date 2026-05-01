import type { SkillGroup, SkillOption } from '../types/chat'

export function getApiBaseUrl(): string {
  const config = (window as any).SUPERBIZAGENT_CONFIG || {}
  return (config.apiBaseUrl || './api').replace(/\/+$/, '')
}

export function getSiteRecord(): { icpNumber: string; icpLink: string } | null {
  const config = (window as any).SUPERBIZAGENT_CONFIG || {}
  const siteRecord = config.siteRecord || {}
  const icpNumber = String(siteRecord.icpNumber || '').trim()
  if (!icpNumber) {
    return null
  }
  const icpLink = String(siteRecord.icpLink || 'https://beian.miit.gov.cn/').trim() || 'https://beian.miit.gov.cn/'
  return { icpNumber, icpLink }
}

export function generateId(): string {
  return crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ')
}

export const SKILL_GROUPS: SkillGroup[] = [
  {
    id: 'metrics',
    label: 'Metrics',
    description: '优先判断影响范围、告警级别和趋势变化。',
    skills: [
      {
        id: 'metrics_alert_triage',
        label: '告警分诊',
        description: '先看核心指标是否越过阈值。',
        domain: 'metrics',
        promptFocus: '优先输出告警级别、受影响指标、变化趋势和影响范围判断。',
      },
      {
        id: 'metrics_incident_snapshot',
        label: '事故快照',
        description: '快速拉齐核心时段内的指标面貌。',
        domain: 'metrics',
        promptFocus: '优先整理近一段时间的延迟、错误率、吞吐和重试变化，形成事故快照。',
      },
      {
        id: 'metrics_release_guard',
        label: '发布回看',
        description: '把异常和最近发布、变更窗口放在一起看。',
        domain: 'metrics',
        promptFocus: '优先判断指标异常是否与最近发布、配置变更或依赖切换相关。',
      },
      {
        id: 'metrics_capacity_snapshot',
        label: '容量压测',
        description: '关注容量瓶颈和资源耗尽信号。',
        domain: 'metrics',
        promptFocus: '优先判断是否存在容量瓶颈、资源耗尽或流量突增导致的异常。',
      },
    ],
  },
  {
    id: 'logs',
    label: 'Logs',
    description: '优先对齐报错路径、异常模式和关键日志片段。',
    skills: [
      {
        id: 'logs_payment_timeout_trace',
        label: '超时追踪',
        description: '顺着 timeout 和 retry 相关日志定位链路。',
        domain: 'logs',
        promptFocus: '优先检查 timeout、retry、context canceled 等异常在核心链路上的集中情况。',
      },
      {
        id: 'logs_evidence_extract',
        label: '证据提取',
        description: '把分散日志整理成可引用的证据项。',
        domain: 'logs',
        promptFocus: '优先提取能支撑结论的错误日志、调用路径和时间窗口证据。',
      },
      {
        id: 'logs_raw_review',
        label: '原始日志审阅',
        description: '保留原始上下文，减少过早归纳。',
        domain: 'logs',
        promptFocus: '优先保留原始日志上下文，再进行归纳，避免过早下结论。',
      },
      {
        id: 'logs_api_failure_rate_investigation',
        label: '失败率排查',
        description: '聚焦失败率抬升和接口错误分布。',
        domain: 'logs',
        promptFocus: '优先分析失败率升高涉及的接口、错误码和依赖调用模式。',
      },
    ],
  },
  {
    id: 'knowledge',
    label: 'Knowledge',
    description: '优先补 SOP、历史案例和标准处置步骤。',
    skills: [
      {
        id: 'knowledge_sop_lookup',
        label: 'SOP 检索',
        description: '优先匹配已有排障 SOP。',
        domain: 'knowledge',
        promptFocus: '优先检索相关 SOP、排查手册和标准操作步骤。',
      },
      {
        id: 'knowledge_incident_guidance',
        label: '案例参考',
        description: '补充相似历史事故的处理思路。',
        domain: 'knowledge',
        promptFocus: '优先引用历史相似案例的判断思路、处理路径和验证方式。',
      },
      {
        id: 'knowledge_release_sop',
        label: '发布检查',
        description: '关注发布后常见异常与回滚条件。',
        domain: 'knowledge',
        promptFocus: '优先结合发布 SOP 判断是否需要回滚、回滚前后要验证什么。',
      },
      {
        id: 'knowledge_rollback_runbook',
        label: '回滚 Runbook',
        description: '优先给出可执行的回滚与验证步骤。',
        domain: 'knowledge',
        promptFocus: '优先输出可执行的回滚步骤、风险提示和验证项。',
      },
    ],
  },
]

export function getSkillCatalog(): SkillOption[] {
  return SKILL_GROUPS.flatMap((group) => group.skills)
}

export function findSkillsByIds(selectedSkillIds: string[]): SkillOption[] {
  if (!Array.isArray(selectedSkillIds) || selectedSkillIds.length === 0) {
    return []
  }
  const validIds = new Set(selectedSkillIds)
  return getSkillCatalog().filter((skill) => validIds.has(skill.id))
}

export function formatSelectedSkillSummary(selectedSkillIds: string[]): string {
  const selected = findSkillsByIds(selectedSkillIds)
  if (selected.length === 0) {
    return '未限定能力范围'
  }
  if (selected.length <= 3) {
    return selected.map((skill) => skill.label).join(' / ')
  }
  return `${selected.slice(0, 3).map((skill) => skill.label).join(' / ')} 等 ${selected.length} 项`
}

export function buildSkillAwareQuery(query: string, selectedSkillIds: string[]): string {
  const trimmed = String(query || '').trim()
  if (!trimmed) {
    return ''
  }
  const selected = findSkillsByIds(selectedSkillIds)
  if (selected.length === 0) {
    return trimmed
  }
  const focus = selected.map((skill) => `- ${skill.label}：${skill.promptFocus}`).join('\n')
  return `请按以下分析重点优先组织本轮回答：\n${focus}\n\n用户问题：${trimmed}`
}
