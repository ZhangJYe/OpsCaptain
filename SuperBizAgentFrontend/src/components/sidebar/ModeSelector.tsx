import { motion } from 'framer-motion'
import { Activity, GitBranch, MessageSquare, Network, Route, Settings2, Zap } from 'lucide-react'
import type { AIOpsEngine, ChatMode, WorkMode } from '../../types/chat'

interface Props {
  value: ChatMode
  onChange: (m: ChatMode) => void
  workMode: WorkMode
  onWorkModeChange: (mode: WorkMode) => void
  aiOpsEngine: AIOpsEngine
  onAIOpsEngineChange: (engine: AIOpsEngine) => void
}

const MODES: { id: ChatMode; label: string; icon: typeof Zap }[] = [
  { id: 'quick', label: '快速', icon: Zap },
  { id: 'stream', label: '流式', icon: GitBranch },
]

const WORK_MODES: { id: WorkMode; label: string; icon: typeof Activity }[] = [
  { id: 'react', label: '问答', icon: MessageSquare },
  { id: 'aiops', label: '事故排障', icon: Activity },
]

const AIOPS_ENGINES: { id: AIOpsEngine; label: string; icon: typeof Route }[] = [
  { id: 'plan_execute_replan', label: 'Plan', icon: Route },
  { id: 'gos_engine', label: 'GoS', icon: Network },
]

export function ModeSelector({ value, onChange, workMode, onWorkModeChange, aiOpsEngine, onAIOpsEngineChange }: Props) {
  return (
    <div className="space-y-3 rounded-xl border border-zinc-200/80 bg-white/80 p-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <div>
        <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">工作模式</p>
        <div className="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
          {WORK_MODES.map((mode) => (
            <button
              key={mode.id}
              onClick={() => onWorkModeChange(mode.id)}
              className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
                workMode === mode.id
                  ? 'text-zinc-900 dark:text-white'
                  : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
              }`}
            >
              {workMode === mode.id && (
                <motion.div
                  layoutId="sidebar-work-mode"
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

      {workMode === 'react' ? (
        <div>
          <p className="mb-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">问答输出</p>
          <div className="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
            {MODES.map((mode) => (
              <button
                key={mode.id}
                onClick={() => onChange(mode.id)}
                className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
                  value === mode.id
                    ? 'text-zinc-900 dark:text-white'
                    : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
                }`}
              >
                {value === mode.id && (
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
      ) : (
        <div className="rounded-lg border border-zinc-200/80 bg-zinc-50/70 px-2.5 py-2 dark:border-zinc-800/80 dark:bg-zinc-950/30">
          <div className="flex items-center justify-between gap-2 text-[11px] font-medium text-zinc-500 dark:text-zinc-400">
            <span className="inline-flex items-center gap-1.5">
              <Settings2 size={13} />
              排障高级设置
            </span>
            <span>{aiOpsEngine === 'gos_engine' ? 'GoS' : 'Plan'}</span>
          </div>
          <div className="mt-2 flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
            {AIOPS_ENGINES.map((engine) => (
              <button
                key={engine.id}
                onClick={() => onAIOpsEngineChange(engine.id)}
                title={engine.id === 'gos_engine' ? '假设、证据、置信度链路' : '计划、执行、重规划链路'}
                className={`relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-xs font-medium transition-colors ${
                  aiOpsEngine === engine.id
                    ? 'text-zinc-900 dark:text-white'
                    : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
                }`}
              >
                {aiOpsEngine === engine.id && (
                  <motion.div
                    layoutId="sidebar-aiops-engine"
                    className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-zinc-200/60 dark:bg-zinc-700 dark:ring-zinc-600/60"
                    transition={{ type: 'spring', damping: 20, stiffness: 300 }}
                  />
                )}
                <engine.icon size={14} className="relative z-10" />
                <span className="relative z-10">{engine.label}</span>
              </button>
            ))}
          </div>
          <p className="mt-2 text-[10px] text-zinc-400 dark:text-zinc-600">
            {aiOpsEngine === 'gos_engine' ? '假设 -> 证据 -> 置信度' : '计划 -> 执行 -> 重规划'}
          </p>
        </div>
      )}
    </div>
  )
}
