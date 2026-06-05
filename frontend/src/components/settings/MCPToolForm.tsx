import React, { useState } from 'react'
import { ArrowLeft, ArrowRight, Check, X } from 'lucide-react'
import type { UserMCPTool } from '../../types/userTools'

interface Props {
  onSubmit: (tool: Partial<UserMCPTool>) => Promise<void>
  onCancel: () => void
}

export function MCPToolForm({ onSubmit, onCancel }: Props) {
  const [step, setStep] = useState(1)
  const [submitting, setSubmitting] = useState(false)

  // Step 1 fields
  const [transport, setTransport] = useState<'sse' | 'http'>('sse')
  const [endpointUrl, setEndpointUrl] = useState('')
  const [httpUrl, setHttpUrl] = useState('')
  const [authToken, setAuthToken] = useState('')

  // Step 2 fields
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [toolName, setToolName] = useState('')
  const [timeoutMs, setTimeoutMs] = useState(30000)

  const step1Valid = endpointUrl.trim().length > 0
  const step2Valid = name.trim().length > 0 && toolName.trim().length > 0

  const handleSubmit = async () => {
    if (!step2Valid) return
    setSubmitting(true)
    try {
      await onSubmit({
        transport,
        endpoint_url: endpointUrl.trim(),
        http_url: httpUrl.trim() || undefined,
        auth_token: authToken.trim() || undefined,
        name: name.trim(),
        description: description.trim(),
        tool_name: toolName.trim(),
        timeout_ms: timeoutMs,
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="border border-white/60 bg-white/70 backdrop-blur-2xl rounded-2xl p-6 space-y-5">
      {/* Step indicator */}
      <div className="flex items-center gap-3 mb-2">
        {[1, 2].map((s) => (
          <div key={s} className="flex items-center gap-2">
            <div
              className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-semibold ${
                s === step
                  ? 'bg-sky-500 text-white'
                  : s < step
                    ? 'bg-sky-200 text-sky-700'
                    : 'bg-gray-200 text-gray-500'
              }`}
            >
              {s < step ? <Check className="w-3.5 h-3.5" /> : s}
            </div>
            <span className={`text-sm ${s === step ? 'text-sky-600 font-medium' : 'text-gray-400'}`}>
              {s === 1 ? '传输配置' : '工具信息'}
            </span>
            {s < 2 && <div className="w-8 h-px bg-gray-300 mx-1" />}
          </div>
        ))}
      </div>

      {step === 1 && (
        <div className="space-y-4">
          {/* Transport toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">传输方式</label>
            <div className="flex gap-2">
              {(['sse', 'http'] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setTransport(t)}
                  className={`px-4 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                    transport === t
                      ? 'bg-sky-500 text-white'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                  }`}
                >
                  {t.toUpperCase()}
                </button>
              ))}
            </div>
          </div>

          {/* Endpoint URL */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              Endpoint URL <span className="text-red-400">*</span>
            </label>
            <input
              type="url"
              value={endpointUrl}
              onChange={(e) => setEndpointUrl(e.target.value)}
              placeholder="https://example.com/mcp/sse"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* HTTP Fallback */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">HTTP 回退地址</label>
            <input
              type="url"
              value={httpUrl}
              onChange={(e) => setHttpUrl(e.target.value)}
              placeholder="https://example.com/mcp/http（可选）"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Auth Token */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">Auth Token</label>
            <input
              type="password"
              value={authToken}
              onChange={(e) => setAuthToken(e.target.value)}
              placeholder="Bearer token（可选）"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="space-y-4">
          {/* Name */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              名称 <span className="text-red-400">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My MCP Tool"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">描述</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="工具描述..."
              rows={2}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent resize-none"
            />
          </div>

          {/* Tool Name */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              Tool Name <span className="text-red-400">*</span>
            </label>
            <input
              type="text"
              value={toolName}
              onChange={(e) => setToolName(e.target.value)}
              placeholder="tool_name"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Timeout */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">超时 (ms)</label>
            <input
              type="number"
              value={timeoutMs}
              onChange={(e) => setTimeoutMs(Number(e.target.value) || 30000)}
              min={1000}
              max={300000}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center justify-between pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="flex items-center gap-1.5 px-4 py-2 text-sm text-gray-500 hover:text-gray-700 transition-colors"
        >
          <X className="w-4 h-4" /> 取消
        </button>
        <div className="flex gap-2">
          {step > 1 && (
            <button
              type="button"
              onClick={() => setStep(step - 1)}
              className="flex items-center gap-1.5 px-4 py-2 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
            >
              <ArrowLeft className="w-4 h-4" /> 上一步
            </button>
          )}
          {step < 2 ? (
            <button
              type="button"
              onClick={() => setStep(2)}
              disabled={!step1Valid}
              className="flex items-center gap-1.5 px-5 py-2 text-sm rounded-lg bg-sky-500 text-white hover:bg-sky-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              下一步 <ArrowRight className="w-4 h-4" />
            </button>
          ) : (
            <button
              type="button"
              onClick={handleSubmit}
              disabled={!step2Valid || submitting}
              className="flex items-center gap-1.5 px-5 py-2 text-sm rounded-lg bg-sky-500 text-white hover:bg-sky-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {submitting ? '提交中...' : '创建工具'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
