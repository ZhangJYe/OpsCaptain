import { useCallback, useEffect, useState } from 'react'
import { getApiBaseUrl } from '../lib/utils'

export interface FeishuNotificationConfig {
  enabled: boolean
  webhook_url: string
  min_risk_level: string
  services: string[]
  timeout_ms: number
}

export interface NotificationConfig {
  feishu: FeishuNotificationConfig | null
}

export function useNotifications() {
  const [config, setConfig] = useState<NotificationConfig | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)
  const [isTesting, setIsTesting] = useState(false)

  const fetchConfig = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const base = getApiBaseUrl()
      const res = await fetch(`${base}/notifications/config`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setConfig(data)
    } catch (e: any) {
      setError(e.message || '加载配置失败')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchConfig()
  }, [fetchConfig])

  const testConnection = useCallback(async (webhookURL?: string) => {
    setIsTesting(true)
    setTestResult(null)
    try {
      const base = getApiBaseUrl()
      const res = await fetch(`${base}/notifications/test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel: 'feishu', webhook_url: webhookURL || '' }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setTestResult(data)
    } catch (e: any) {
      setTestResult({ success: false, message: e.message || '测试失败' })
    } finally {
      setIsTesting(false)
    }
  }, [])

  return {
    config,
    isLoading,
    error,
    testResult,
    isTesting,
    testConnection,
    refresh: fetchConfig,
  }
}
