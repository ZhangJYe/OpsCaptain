import React, { useState } from 'react'
import { ArrowLeft, ArrowRight, Check, X } from 'lucide-react'
import type { UserMCPTool, UserSkill } from '../../types/userTools'

interface Props {
  tools: UserMCPTool[]
  onSubmit: (skill: Partial<UserSkill>) => Promise<void>
  onCancel: () => void
}

const DOMAIN_OPTIONS = [
  { value: 'metrics' as const, label: 'Metrics' },
  { value: 'logs' as const, label: 'Logs' },
  { value: 'knowledge' as const, label: 'Knowledge' },
  { value: 'custom' as const, label: 'Custom' },
]

const PARSER_OPTIONS = [
  { value: 'json_array' as const, label: 'JSON Array' },
  { value: 'json_nested' as const, label: 'JSON Nested' },
  { value: 'log_lines' as const, label: 'Log Lines' },
  { value: 'raw' as const, label: 'Raw' },
]

export function UserSkillForm({ tools, onSubmit, onCancel }: Props) {
  const [step, setStep] = useState(1)
  const [submitting, setSubmitting] = useState(false)

  // Step 1 fields
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [domain, setDomain] = useState<UserSkill['domain']>('metrics')
  const [toolRefId, setToolRefId] = useState('')

  // Step 2 fields
  const [keywordInput, setKeywordInput] = useState('')
  const [keywords, setKeywords] = useState<string[]>([])
  const [focus, setFocus] = useState('')
  const [outputParser, setOutputParser] = useState<UserSkill['output_parser']>('json_array')
  const [jsonPath, setJsonPath] = useState('')
  const [tier, setTier] = useState(0)

  const approvedTools = tools.filter((t) => t.status === 'approved')

  const step1Valid = name.trim().length > 0 && toolRefId.length > 0
  const step2Valid = keywords.length > 0

  const handleAddKeyword = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      const kw = keywordInput.trim()
      if (kw && !keywords.includes(kw)) {
        setKeywords([...keywords, kw])
      }
      setKeywordInput('')
    }
  }

  const handleRemoveKeyword = (kw: string) => {
    setKeywords(keywords.filter((k) => k !== kw))
  }

  const handleSubmit = async () => {
    if (!step2Valid) return
    setSubmitting(true)
    try {
      await onSubmit({
        name: name.trim(),
        description: description.trim(),
        domain,
        tool_ref_id: toolRefId,
        keywords,
        focus: focus.trim() || undefined,
        output_parser: outputParser,
        json_path: outputParser === 'json_nested' ? jsonPath.trim() || undefined : undefined,
        tier,
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
              {s === 1 ? '基本信息' : '关键词 & 解析'}
            </span>
            {s < 2 && <div className="w-8 h-px bg-gray-300 mx-1" />}
          </div>
        ))}
      </div>

      {step === 1 && (
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
              placeholder="My Skill"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">描述</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Skill 描述..."
              rows={2}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent resize-none"
            />
          </div>

          {/* Domain toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">领域</label>
            <div className="flex flex-wrap gap-2">
              {DOMAIN_OPTIONS.map((d) => (
                <button
                  key={d.value}
                  type="button"
                  onClick={() => setDomain(d.value)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                    domain === d.value
                      ? 'bg-sky-500 text-white'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
          </div>

          {/* Tool dropdown */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              关联工具 <span className="text-red-400">*</span>
            </label>
            <select
              value={toolRefId}
              onChange={(e) => setToolRefId(e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            >
              <option value="">选择已审批的工具...</option>
              {approvedTools.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} ({t.tool_name})
                </option>
              ))}
            </select>
            {approvedTools.length === 0 && (
              <p className="text-xs text-amber-500 mt-1">暂无已审批的工具，请先创建并审批工具</p>
            )}
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="space-y-4">
          {/* Keywords tag input */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              关键词 <span className="text-red-400">*</span>
            </label>
            <div className="flex flex-wrap gap-1.5 mb-2">
              {keywords.map((kw) => (
                <span
                  key={kw}
                  className="inline-flex items-center gap-1 px-2 py-0.5 bg-sky-100 text-sky-700 rounded-full text-xs"
                >
                  {kw}
                  <button
                    type="button"
                    onClick={() => handleRemoveKeyword(kw)}
                    className="hover:text-sky-900"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </span>
              ))}
            </div>
            <input
              type="text"
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              onKeyDown={handleAddKeyword}
              placeholder="输入关键词后按 Enter 添加"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Focus */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">关注点</label>
            <input
              type="text"
              value={focus}
              onChange={(e) => setFocus(e.target.value)}
              placeholder="e.g. latency, error_rate"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
            />
          </div>

          {/* Output Parser toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">输出解析器</label>
            <div className="flex flex-wrap gap-2">
              {PARSER_OPTIONS.map((p) => (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => setOutputParser(p.value)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                    outputParser === p.value
                      ? 'bg-sky-500 text-white'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          {/* JSON Path (conditional) */}
          {outputParser === 'json_nested' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">JSON Path</label>
              <input
                type="text"
                value={jsonPath}
                onChange={(e) => setJsonPath(e.target.value)}
                placeholder="$.data.results"
                className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-white/80 text-sm focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
              />
            </div>
          )}

          {/* Tier toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">层级</label>
            <div className="flex gap-2">
              {[
                { value: 0, label: 'SkillGate' },
                { value: 1, label: 'OnDemand' },
              ].map((t) => (
                <button
                  key={t.value}
                  type="button"
                  onClick={() => setTier(t.value)}
                  className={`px-4 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                    tier === t.value
                      ? 'bg-sky-500 text-white'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>
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
              {submitting ? '提交中...' : '创建 Skill'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
