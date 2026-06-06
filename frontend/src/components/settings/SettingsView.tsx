import React, { useState } from 'react'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useUserTools } from '../../hooks/useUserTools'
import { ToolManager } from './ToolManager'
import { SkillManager } from './SkillManager'

interface Props {
  onBack: () => void
}

const TABS = [
  { id: 'tools' as const, label: 'MCP 工具' },
  { id: 'skills' as const, label: 'Skill' },
]

export function SettingsView({ onBack }: Props) {
  const [activeTab, setActiveTab] = useState<'tools' | 'skills'>('tools')
  const {
    tools,
    skills,
    isLoading,
    error,
    createTool,
    deleteTool,
    testTool,
    approveTool,
    rejectTool,
    createSkill,
    deleteSkill,
    approveSkill,
    rejectSkill,
  } = useUserTools()

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-6 py-4 border-b border-white/40">
        <button
          onClick={onBack}
          className="flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-800 transition-colors"
        >
          <ArrowLeft className="w-4 h-4" />
          返回
        </button>
        <h1 className="text-lg font-semibold text-gray-900">工具 & Skill 管理</h1>
      </div>

      {/* Tab bar */}
      <div className="flex gap-0 px-6 border-b border-white/40">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`relative px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === tab.id
                ? 'text-sky-600'
                : 'text-gray-400 hover:text-gray-600'
            }`}
          >
            {tab.label}
            {activeTab === tab.id && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-sky-500 rounded-full" />
            )}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-6 py-5">
        {isLoading && (
          <div className="flex items-center justify-center py-16 text-gray-400">
            <Loader2 className="w-5 h-5 animate-spin mr-2" />
            加载中...
          </div>
        )}

        {error && (
          <div className="bg-red-50 text-red-600 text-sm px-4 py-3 rounded-xl mb-4">
            {error}
          </div>
        )}

        {!isLoading && !error && activeTab === 'tools' && (
          <ToolManager
            tools={tools}
            onCreate={createTool}
            onDelete={deleteTool}
            onTest={testTool}
          />
        )}

        {!isLoading && !error && activeTab === 'skills' && (
          <SkillManager
            skills={skills}
            tools={tools}
            onCreate={createSkill}
            onDelete={deleteSkill}
          />
        )}
      </div>
    </div>
  )
}
