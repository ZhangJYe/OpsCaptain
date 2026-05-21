import { motion } from 'framer-motion'
import { Activity, GitBranch, MessageSquare, Network, Route, Zap } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { AIOpsEngine, ChatMode, WorkbenchMode } from '../../types/chat'
import { ENGINE_VIEW_MODEL } from '../../lib/engineViewModel'

interface Props {
  value: ChatMode
  onChange: (m: ChatMode) => void
  workbenchMode: WorkbenchMode
  onWorkbenchModeChange: (mode: WorkbenchMode) => void
  aiOpsEngine: AIOpsEngine
  onAIOpsEngineChange: (engine: AIOpsEngine) => void
}

const WORKBENCH_MODES: { id: WorkbenchMode; label: string; icon: typeof Activity }[] = [
  { id: 'aiops', label: 'AIOps 排障', icon: Activity },
  { id: 'chat', label: 'ReAct 问答', icon: MessageSquare },
]

const MODES: { id: ChatMode; label: string; icon: typeof Zap }[] = [
  { id: 'quick', label: '快速', icon: Zap },
  { id: 'stream', label: '流式', icon: GitBranch },
]

const AIOPS_ENGINES: AIOpsEngine[] = ['plan_execute_replan', 'gos_engine']

const AIOPS_ENGINE_ICONS: Record<AIOpsEngine, LucideIcon> = {
  plan_execute_replan: Route,
  gos_engine: Network,
}

export function ModeSelector({ value, onChange, workbenchMode, onWorkbenchModeChange, aiOpsEngine, onAIOpsEngineChange }: Props) {
  return (
    <div className="space-y-3 rounded-xl border border-zinc-200/80 bg-white/80 p-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <div>
        <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">工作模式</p>
        <div className="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
          {WORKBENCH_MODES.map((mode) => (
            <button
              key={mode.id}
              onClick={() => onWorkbenchModeChange(mode.id)}
              className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
                workbenchMode === mode.id
                  ? 'text-zinc-900 dark:text-white'
                  : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
              }`}
            >
              {workbenchMode === mode.id && (
                <motion.div
                  layoutId="sidebar-workbench-mode"
                  className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-zinc-200/60 dark:bg-zinc-700 dark:ring-zinc-600/60"
                  transition={{ type: 'spring', damping: 20, stiffness: 300 }}
                />
              )}
              <mode.icon size={14} className="relative z-10" />
              <span className="relative z-10">{mode.label}</span>
            </button>
          ))}
        </div>
      </div>

      <div>
        <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">ReAct 输出</p>
        <div className="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
          {MODES.map((mode) => (
            <button
              key={mode.id}
              onClick={() => {
                onWorkbenchModeChange('chat')
                onChange(mode.id)
              }}
              className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
                workbenchMode === 'chat' && value === mode.id
                  ? 'text-zinc-900 dark:text-white'
                  : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
              }`}
            >
              {workbenchMode === 'chat' && value === mode.id && (
                <motion.div
                  layoutId="sidebar-mode"
                  className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-zinc-200/60 dark:bg-zinc-700 dark:ring-zinc-600/60"
                  transition={{ type: 'spring', damping: 20, stiffness: 300 }}
                />
              )}
              <mode.icon size={14} className="relative z-10" />
              <span className="relative z-10">{mode.label}</span>
            </button>
          ))}
        </div>
      </div>

      <div>
        <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">AIOps 引擎</p>
        <div className="grid gap-2">
          {AIOPS_ENGINES.map((engine) => {
            const view = ENGINE_VIEW_MODEL[engine]
            const Icon = AIOPS_ENGINE_ICONS[engine]
            const selected = workbenchMode === 'aiops' && aiOpsEngine === engine

            return (
              <button
                key={engine}
                onClick={() => {
                  onWorkbenchModeChange('aiops')
                  onAIOpsEngineChange(engine)
                }}
                title={view.trace}
                className={`relative overflow-hidden rounded-xl border p-2.5 text-left transition-all duration-200 ${selected ? view.sidebar.selected : view.sidebar.idle}`}
              >
                {selected && (
                  <motion.div
                    layoutId="sidebar-aiops-engine"
                    className="absolute inset-0 rounded-xl bg-white/35 dark:bg-white/[0.03]"
                    transition={{ type: 'spring', damping: 22, stiffness: 300 }}
                  />
                )}
                <div className="relative flex items-start gap-2.5">
                  <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ring-1 ${view.sidebar.icon}`}>
                    <Icon size={15} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center justify-between gap-2">
                      <span className="text-sm font-semibold">{view.label}</span>
                      {selected && <span className={`h-1.5 w-1.5 rounded-full ${view.sidebar.dot}`} />}
                    </span>
                    <span className="mt-0.5 block text-[10px] leading-4 text-zinc-500 dark:text-zinc-500">
                      {view.description}
                    </span>
                  </span>
                </div>
                <div className="relative mt-2 grid grid-cols-3 gap-1">
                  {view.flow.map((item) => (
                    <span
                      key={item}
                      className={`truncate rounded-md px-1.5 py-1 text-center text-[9px] font-semibold ring-1 ${
                        selected
                          ? view.sidebar.flowActive
                          : 'bg-white/50 text-zinc-400 ring-white/60 dark:bg-slate-700/40 dark:text-zinc-500 dark:ring-white/10'
                      }`}
                    >
                      {item}
                    </span>
                  ))}
                </div>
              </button>
            )
          })}
        </div>
        <p className="mt-2 text-[10px] text-zinc-400 dark:text-zinc-600">
          {workbenchMode === 'aiops' ? ENGINE_VIEW_MODEL[aiOpsEngine].trace : '当前输入走 ReAct'}
        </p>
      </div>
    </div>
  )
}
