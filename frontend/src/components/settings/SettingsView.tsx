import React, { useState } from 'react'
import { Activity, ArrowLeft, Bot, Check, GitBranch, Loader2, Route } from 'lucide-react'
import { useUserTools } from '../../hooks/useUserTools'
import { useNotifications } from '../../hooks/useNotifications'
import { ToolManager } from './ToolManager'
import { SkillManager } from './SkillManager'
import { FeishuNotificationConfigView } from './FeishuNotificationConfig'
import type { AgentDiagnosisStrategy, AgentRouteMode, AgentRuntimeProfile } from '../../types/chat'

interface Props {
  onBack: () => void
  runtimeProfile: AgentRuntimeProfile
  onRuntimeProfileChange: (profile: AgentRuntimeProfile) => void
  activeIncidentEngine: string
}

const TABS = [
  { id: 'runtime' as const, label: '运行配置' },
  { id: 'tools' as const, label: 'MCP 工具' },
  { id: 'skills' as const, label: 'Skill' },
  { id: 'notifications' as const, label: '通知' },
]

const routeOptions: Array<{ id: AgentRouteMode; title: string; description: string; Icon: typeof Route }> = [
  { id: 'auto', title: '自动路由', description: 'Flash 先判断问答或真实故障，再进入对应链路。', Icon: Route },
  { id: 'react', title: 'ReAct 问答', description: '直接进入流式问答，不会主动创建事故。', Icon: Bot },
  { id: 'diagnosis', title: '故障诊断', description: '下一次请求直接创建事故，并锁定实际策略。', Icon: Activity },
]

const strategyOptions: Array<{ id: AgentDiagnosisStrategy; title: string; description: string }> = [
  { id: 'auto', title: '自动推荐', description: 'Flash 根据故障路径与根因数量推荐 Plan 或 GoS。' },
  { id: 'plan_execute_replan', title: 'Plan', description: '按 runbook 线性推进：计划、执行、重规划。' },
  { id: 'gos_engine', title: 'GoS', description: '围绕候选根因组织支持、反驳与置信度。' },
]

function RuntimeConfigPanel({ runtimeProfile, onRuntimeProfileChange: onChange, activeIncidentEngine }: Pick<Props, 'runtimeProfile' | 'onRuntimeProfileChange' | 'activeIncidentEngine'>) {
  const flow = runtimeProfile.routeMode === 'react'
    ? ['请求', 'ReAct', '流式回答']
    : runtimeProfile.routeMode === 'diagnosis'
      ? ['请求', runtimeProfile.diagnosisStrategy === 'auto' ? 'Flash 推荐' : runtimeProfile.diagnosisStrategy === 'gos_engine' ? 'GoS' : 'Plan', '事故诊断']
      : ['请求', 'Flash 分流', runtimeProfile.diagnosisStrategy === 'auto' ? 'ReAct / Plan / GoS' : `ReAct / ${runtimeProfile.diagnosisStrategy === 'gos_engine' ? 'GoS' : 'Plan'}`]

  return (
    <section className="mx-auto max-w-4xl space-y-6 pb-10">
      <div>
        <p className="text-xs font-semibold tracking-[0.14em] text-sky-600">AGENT RUNTIME</p>
        <h2 className="mt-1 text-xl font-semibold tracking-tight text-zinc-900 dark:text-white">下一次请求如何运行</h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-500 dark:text-zinc-400">Flash 只做意图识别与策略建议；ReAct 负责问答，Plan 与 GoS 是互斥的诊断策略。已创建的事故不会被这里的修改影响。</p>
      </div>

      <div className="grid gap-3 md:grid-cols-3">
        {routeOptions.map(({ id, title, description, Icon }) => {
          const selected = runtimeProfile.routeMode === id
          return (
            <button key={id} type="button" onClick={() => onChange({ ...runtimeProfile, routeMode: id })} className={`group rounded-2xl border p-4 text-left transition ${selected ? 'border-sky-300 bg-sky-50/80 shadow-sm shadow-sky-500/10 dark:border-sky-500/40 dark:bg-sky-500/10' : 'border-white/70 bg-white/55 hover:border-sky-200 hover:bg-sky-50/40 dark:border-white/10 dark:bg-slate-800/35 dark:hover:border-sky-500/25'}`}>
              <div className="flex items-start justify-between gap-3">
                <span className={`flex h-9 w-9 items-center justify-center rounded-xl ${selected ? 'bg-sky-500 text-white' : 'bg-zinc-100 text-zinc-500 dark:bg-slate-700 dark:text-zinc-400'}`}><Icon size={17} /></span>
                {selected && <span className="flex h-5 w-5 items-center justify-center rounded-full bg-sky-500 text-white"><Check size={13} /></span>}
              </div>
              <p className="mt-4 text-sm font-semibold text-zinc-800 dark:text-zinc-100">{title}</p>
              <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">{description}</p>
            </button>
          )
        })}
      </div>

      {runtimeProfile.routeMode !== 'react' && (
        <div className="rounded-2xl border border-white/70 bg-white/55 p-5 dark:border-white/10 dark:bg-slate-800/35">
          <div className="flex items-center gap-2"><GitBranch size={16} className="text-sky-500" /><h3 className="text-sm font-semibold text-zinc-800 dark:text-zinc-100">故障诊断策略</h3></div>
          <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">自动路由识别为故障后，或你显式启动排障时，使用下面的偏好。</p>
          <div className="mt-4 grid gap-2 sm:grid-cols-3">
            {strategyOptions.map(({ id, title, description }) => {
              const selected = runtimeProfile.diagnosisStrategy === id
              return <button key={id} type="button" onClick={() => onChange({ ...runtimeProfile, diagnosisStrategy: id })} className={`rounded-xl border px-3 py-3 text-left transition ${selected ? id === 'gos_engine' ? 'border-amber-300 bg-amber-50/80 dark:border-amber-500/40 dark:bg-amber-500/10' : 'border-sky-300 bg-sky-50/80 dark:border-sky-500/40 dark:bg-sky-500/10' : 'border-zinc-100 bg-white/40 hover:border-zinc-200 dark:border-white/5 dark:bg-slate-900/20'}`}>
                <p className="text-xs font-semibold text-zinc-800 dark:text-zinc-100">{title}</p><p className="mt-1 text-[11px] leading-4 text-zinc-500 dark:text-zinc-400">{description}</p>
              </button>
            })}
          </div>
        </div>
      )}

      <div className="rounded-2xl border border-slate-200/70 bg-slate-50/80 p-4 dark:border-white/10 dark:bg-slate-900/35">
        <div className="flex items-center justify-between gap-3"><p className="text-xs font-semibold text-zinc-700 dark:text-zinc-200">本次配置的执行预览</p><span className="text-[10px] text-zinc-400">仅对新请求生效</span></div>
        <div className="mt-3 flex flex-wrap items-center gap-2">{flow.map((item, index) => <React.Fragment key={item}><span className={`rounded-lg px-2.5 py-1.5 text-xs font-medium ${index === flow.length - 1 ? 'bg-sky-500 text-white' : 'bg-white text-zinc-600 ring-1 ring-zinc-200/70 dark:bg-slate-800 dark:text-zinc-300 dark:ring-white/10'}`}>{item}</span>{index < flow.length - 1 && <span className="text-zinc-300">→</span>}</React.Fragment>)}</div>
      </div>

      {activeIncidentEngine && <div className="rounded-xl border border-amber-200 bg-amber-50/70 px-4 py-3 text-xs leading-5 text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200">当前事故已锁定为 {activeIncidentEngine === 'gos_engine' ? 'GoS' : 'Plan'}；修改运行配置只会影响下一次新请求。</div>}
    </section>
  )
}

export function SettingsView({ onBack, runtimeProfile, onRuntimeProfileChange, activeIncidentEngine }: Props) {
  const [activeTab, setActiveTab] = useState<'runtime' | 'tools' | 'skills' | 'notifications'>('runtime')
  const {
    tools,
    skills,
    isLoading: isToolsLoading,
    error: toolsError,
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

  const {
    config: notifConfig,
    isLoading: isNotifLoading,
    error: notifError,
    testResult,
    isTesting,
    testConnection,
  } = useNotifications()

  const isLoading = activeTab === 'notifications' ? isNotifLoading : activeTab === 'runtime' ? false : isToolsLoading
  const error = activeTab === 'notifications' ? notifError : activeTab === 'runtime' ? null : toolsError

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
        <h1 className="text-lg font-semibold text-gray-900 dark:text-white">运行配置与能力管理</h1>
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

        {activeTab === 'runtime' && <RuntimeConfigPanel runtimeProfile={runtimeProfile} onRuntimeProfileChange={onRuntimeProfileChange} activeIncidentEngine={activeIncidentEngine} />}

        {!isLoading && !error && activeTab === 'skills' && (
          <SkillManager
            skills={skills}
            tools={tools}
            onCreate={createSkill}
            onDelete={deleteSkill}
          />
        )}

        {!isLoading && !error && activeTab === 'notifications' && (
          <FeishuNotificationConfigView
            config={notifConfig?.feishu || null}
            isTesting={isTesting}
            testResult={testResult}
            onTest={testConnection}
          />
        )}
      </div>
    </div>
  )
}
