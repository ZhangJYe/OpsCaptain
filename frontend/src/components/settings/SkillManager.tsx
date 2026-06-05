import React, { useState } from 'react'
import { Plus, Trash2, CheckCircle, XCircle, Puzzle } from 'lucide-react'
import type { UserMCPTool, UserSkill } from '../../types/userTools'
import { ApprovalBadge } from './ApprovalBadge'
import { UserSkillForm } from './UserSkillForm'

interface Props {
  skills: UserSkill[]
  tools: UserMCPTool[]
  onCreate: (skill: Partial<UserSkill>) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onApprove: (id: string) => Promise<void>
  onReject: (id: string) => Promise<void>
}

export function SkillManager({ skills, tools, onCreate, onDelete, onApprove, onReject }: Props) {
  const [showForm, setShowForm] = useState(false)

  const getToolName = (toolRefId: string) => {
    const tool = tools.find((t) => t.id === toolRefId)
    return tool ? tool.name : toolRefId
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-500 uppercase tracking-wide">
          Skill ({skills.length})
        </h3>
        <button
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg bg-sky-500 text-white hover:bg-sky-600 transition-colors"
        >
          <Plus className="w-4 h-4" /> 添加 Skill
        </button>
      </div>

      {showForm && (
        <UserSkillForm
          tools={tools}
          onSubmit={async (skill) => {
            await onCreate(skill)
            setShowForm(false)
          }}
          onCancel={() => setShowForm(false)}
        />
      )}

      {skills.length === 0 && !showForm && (
        <div className="text-center py-12 text-gray-400 text-sm">
          暂无 Skill，点击上方按钮添加
        </div>
      )}

      <div className="grid gap-3">
        {skills.map((skill) => (
          <div
            key={skill.id}
            className="border border-white/60 bg-white/70 backdrop-blur-2xl rounded-xl p-4 space-y-2"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 mb-1">
                  <Puzzle className="w-4 h-4 text-sky-500 shrink-0" />
                  <span className="font-medium text-gray-900 truncate">{skill.name}</span>
                  <ApprovalBadge status={skill.status} />
                </div>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
                  <span>领域: {skill.domain}</span>
                  <span>工具: {getToolName(skill.tool_ref_id)}</span>
                  <span>解析: {skill.output_parser}</span>
                  <span>层级: {skill.tier === 0 ? 'SkillGate' : 'OnDemand'}</span>
                </div>
                {skill.keywords.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1.5">
                    {skill.keywords.map((kw) => (
                      <span
                        key={kw}
                        className="px-1.5 py-0.5 bg-sky-50 text-sky-600 rounded text-xs"
                      >
                        {kw}
                      </span>
                    ))}
                  </div>
                )}
                {skill.description && (
                  <p className="text-xs text-gray-400 mt-1 line-clamp-2">{skill.description}</p>
                )}
              </div>

              <div className="flex items-center gap-1.5 shrink-0">
                {skill.status === 'pending' && (
                  <>
                    <button
                      onClick={() => onApprove(skill.id)}
                      title="审批"
                      className="p-1.5 rounded-lg text-green-500 hover:bg-green-50 transition-colors"
                    >
                      <CheckCircle className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => onReject(skill.id)}
                      title="拒绝"
                      className="p-1.5 rounded-lg text-amber-500 hover:bg-amber-50 transition-colors"
                    >
                      <XCircle className="w-4 h-4" />
                    </button>
                  </>
                )}
                <button
                  onClick={() => onDelete(skill.id)}
                  title="删除"
                  className="p-1.5 rounded-lg text-red-400 hover:bg-red-50 transition-colors"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
