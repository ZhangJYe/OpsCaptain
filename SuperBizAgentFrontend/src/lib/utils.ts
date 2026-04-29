export function getApiBaseUrl(): string {
  const config = (window as any).SUPERBIZAGENT_CONFIG || {}
  return (config.apiBaseUrl || './api').replace(/\/+$/, '')
}

export function generateId(): string {
  return crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ')
}
