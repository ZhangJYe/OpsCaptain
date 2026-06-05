import React, { useState } from 'react'
import { Loader2, Plus, TestTube2, Trash2, CheckCircle, XCircle } from 'lucide-react'
import type { UserMCPTool } from '../../types/userTools'
import { ApprovalBadge } from './ApprovalBadge'
import { MCPToolForm } from './MCPToolForm'

interface Props {
  tools: UserMCPTool[]
  onCreate: (tool: Partial<UserMCPTool>) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onTest: (id: string) => Promise<{ ok: boolean; message: string }>
  onApprove: (id: string) => Promise<void>
  onReject: (id: string) => Promise<void>
}

export function ToolManager({ tools, onCreate, onDelete, onTest, onApprove, onReject }: Props) {
  const [showForm, setShowForm] = useState(false)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; message: string }>>({})

  const handleTest = async (id: string) => {
    setTestingId(id)
    try {
      const result = await onTest(id)
      setTestResults((prev) => ({ ...prev, [id]: result }))
    } finally {
      setTestingId(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-500 uppercase tracking-wide">
          MCP 工具 ({tools.length})
        </h3>
        <button
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg bg-sky-500 text-white hover:bg-sky-600 transition-colors"
        >
          <Plus className="w-4 h-4" /> 添加 MCP 工具
        </button>
      </div>

      {showForm && (
        <MCPToolForm
          onSubmit={async (tool) => {
            await onCreate(tool)
            setShowForm(false)
          }}
          onCancel={() => setShowForm(false)}
        />
      )}

      {tools.length === 0 && !showForm && (
        <div className="text-center py-12 text-gray-400 text-sm">
          暂无 MCP 工具，点击上方按钮添加
        </div>
      )}

      <div className="grid gap-3">
        {tools.map((tool) => {
          const testResult = testResults[tool.id]
          return (
            <div
              key={tool.id}
              className="border border-white/60 bg-white/70 backdrop-blur-2xl rounded-xl p-4 space-y-3"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="font-medium text-gray-900 truncate">{tool.name}</span>
                    <ApprovalBadge status={tool.status} />
                  </div>
                  <p className="text-xs text-gray-500 truncate">
                    {tool.transport.toUpperCase()} &middot; {tool.endpoint_url}
                  </p>
                  {tool.description && (
                    <p className="text-xs text-gray-400 mt-1 line-clamp-2">{tool.description}</p>
                  )}
                </div>

                <div className="flex items-center gap-1.5 shrink-0">
                  <button
                    onClick={() => handleTest(tool.id)}
                    disabled={testingId === tool.id}
                    title="测试"
                    className="p-1.5 rounded-lg text-sky-500 hover:bg-sky-50 disabled:opacity-40 transition-colors"
                  >
                    {testingId === tool.id ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <TestTube2 className="w-4 h-4" />
                    )}
                  </button>
                  {tool.status === 'pending' && (
                    <>
                      <button
                        onClick={() => onApprove(tool.id)}
                        title="审批"
                        className="p-1.5 rounded-lg text-green-500 hover:bg-green-50 transition-colors"
                      >
                        <CheckCircle className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => onReject(tool.id)}
                        title="拒绝"
                        className="p-1.5 rounded-lg text-amber-500 hover:bg-amber-50 transition-colors"
                      >
                        <XCircle className="w-4 h-4" />
                      </button>
                    </>
                  )}
                  <button
                    onClick={() => onDelete(tool.id)}
                    title="删除"
                    className="p-1.5 rounded-lg text-red-400 hover:bg-red-50 transition-colors"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              {testResult && (
                <div
                  className={`flex items-center gap-2 text-xs px-3 py-1.5 rounded-lg ${
                    testResult.ok
                      ? 'bg-green-50 text-green-700'
                      : 'bg-red-50 text-red-600'
                  }`}
                >
                  {testResult.ok ? (
                    <CheckCircle className="w-3.5 h-3.5 shrink-0" />
                  ) : (
                    <XCircle className="w-3.5 h-3.5 shrink-0" />
                  )}
                  <span className="truncate">{testResult.message}</span>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
