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
