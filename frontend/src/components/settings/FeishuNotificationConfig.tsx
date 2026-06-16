import { useState } from 'react'
import { Check, ExternalLink, Loader2, Send, AlertCircle } from 'lucide-react'
import type { FeishuNotificationConfig as FeishuConfig } from '../../hooks/useNotifications'

interface Props {
  config: FeishuConfig | null
  isTesting: boolean
  testResult: { success: boolean; message: string } | null
  onTest: (webhookURL?: string) => void
}

const RISK_LEVELS = [
  { value: 'low', label: '低风险 (low)', desc: '推送所有变更' },
  { value: 'medium', label: '中风险 (medium)', desc: '仅推送中风险及以上' },
  { value: 'high', label: '高风险 (high)', desc: '仅推送高风险和严重' },
  { value: 'critical', label: '严重 (critical)', desc: '仅推送严重级别' },
]

export function FeishuNotificationConfigView({ config, isTesting, testResult, onTest }: Props) {
  const [webhookURL, setWebhookURL] = useState(config?.webhook_url || '')
  const [minRiskLevel, setMinRiskLevel] = useState(config?.min_risk_level || 'medium')
  const [services, setServices] = useState((config?.services || []).join(', '))

  const maskedURL = webhookURL
    ? webhookURL.replace(/(bot\/v2\/hook\/).{4}(.{4})/, '$1****$2')
    : ''

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100">飞书通知</h3>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          变更事件发生时，自动发送飞书卡片消息到值班群。
        </p>
      </div>

      {/* Status badge */}
      <div className="flex items-center gap-2">
        <span
          className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${
            config?.enabled
              ? 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-500/20'
              : 'bg-gray-100 text-gray-500 ring-1 ring-gray-200 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700'
          }`}
        >
          <span className={`h-1.5 w-1.5 rounded-full ${config?.enabled ? 'bg-emerald-500' : 'bg-gray-400'}`} />
          {config?.enabled ? '已启用' : '未启用'}
        </span>
      </div>

      {/* Webhook URL */}
      <div className="space-y-2">
        <label className="block text-xs font-medium text-gray-700 dark:text-gray-300">
          Webhook URL
        </label>
        <div className="flex gap-2">
          <input
            type="text"
            value={webhookURL}
            onChange={(e) => setWebhookURL(e.target.value)}
            placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
            className="flex-1 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs text-gray-900 placeholder-gray-400 focus:border-sky-400 focus:outline-none focus:ring-1 focus:ring-sky-400 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500"
          />
        </div>
        {config?.webhook_url && (
          <p className="flex items-center gap-1 text-[10px] text-gray-400 dark:text-gray-500">
            当前配置: <code className="font-mono">{maskedURL}</code>
          </p>
        )}
        <a
          href="https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-[10px] text-sky-500 hover:text-sky-600 dark:text-sky-400"
        >
          如何获取 Webhook URL <ExternalLink size={10} />
        </a>
      </div>

      {/* Risk Level */}
      <div className="space-y-2">
        <label className="block text-xs font-medium text-gray-700 dark:text-gray-300">
          最低推送风险等级
        </label>
        <div className="grid grid-cols-2 gap-2">
          {RISK_LEVELS.map((level) => (
            <button
              key={level.value}
              onClick={() => setMinRiskLevel(level.value)}
              className={`rounded-lg border px-3 py-2 text-left transition-colors ${
                minRiskLevel === level.value
                  ? 'border-sky-300 bg-sky-50 dark:border-sky-500/30 dark:bg-sky-500/10'
                  : 'border-gray-200 bg-white hover:border-gray-300 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-gray-600'
              }`}
            >
              <span
                className={`block text-xs font-medium ${
                  minRiskLevel === level.value
                    ? 'text-sky-700 dark:text-sky-300'
                    : 'text-gray-700 dark:text-gray-300'
                }`}
              >
                {level.label}
              </span>
              <span className="mt-0.5 block text-[10px] text-gray-400 dark:text-gray-500">
                {level.desc}
              </span>
            </button>
          ))}
        </div>
      </div>

      {/* Services filter */}
      <div className="space-y-2">
        <label className="block text-xs font-medium text-gray-700 dark:text-gray-300">
          监控服务
          <span className="ml-1 font-normal text-gray-400">(留空 = 全部服务)</span>
        </label>
        <input
          type="text"
          value={services}
          onChange={(e) => setServices(e.target.value)}
          placeholder="payment-service, order-service"
          className="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs text-gray-900 placeholder-gray-400 focus:border-sky-400 focus:outline-none focus:ring-1 focus:ring-sky-400 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500"
        />
        <p className="text-[10px] text-gray-400 dark:text-gray-500">
          用逗号分隔多个服务名。留空则推送所有服务的变更。
        </p>
      </div>

      {/* Config hint */}
      <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 dark:border-amber-500/20 dark:bg-amber-500/5">
        <div className="flex items-start gap-2">
          <AlertCircle size={14} className="mt-0.5 shrink-0 text-amber-500" />
          <div className="text-[11px] text-amber-700 dark:text-amber-300">
            <p className="font-medium">配置需要重启生效</p>
            <p className="mt-0.5">
              修改 <code className="font-mono">config.yaml</code> 中的{' '}
              <code className="font-mono">change_events.notifier.feishu</code> 后重启服务。
            </p>
          </div>
        </div>
      </div>

      {/* Test connection */}
      <div className="space-y-2">
        <button
          onClick={() => onTest(webhookURL)}
          disabled={isTesting || !webhookURL}
          className="inline-flex items-center gap-2 rounded-lg bg-sky-500 px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-sky-600 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-sky-600 dark:hover:bg-sky-700"
        >
          {isTesting ? (
            <Loader2 size={14} className="animate-spin" />
          ) : (
            <Send size={14} />
          )}
          {isTesting ? '发送中...' : '测试连接'}
        </button>

        {testResult && (
          <div
            className={`flex items-center gap-2 rounded-lg px-3 py-2 text-xs ${
              testResult.success
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
                : 'bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-400'
            }`}
          >
            {testResult.success ? <Check size={14} /> : <AlertCircle size={14} />}
            {testResult.message}
          </div>
        )}
      </div>
    </div>
  )
}
