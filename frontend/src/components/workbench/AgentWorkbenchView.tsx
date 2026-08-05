import { useRef, useEffect, useState, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Bot, BotOff, PanelRightOpen, PanelRightClose, CheckCircle2, CircleAlert, Loader2, Route } from 'lucide-react'
import { MessageBubble } from '../chat/MessageBubble'
import { StreamingText } from '../chat/StreamingText'
import { ChatInput } from '../chat/ChatInput'
import { GoSBeliefProgress } from './GoSBeliefProgress'
import { SuggestionChips } from '../agent/SuggestionChips'
import type { Suggestion } from '../agent/SuggestionChips'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { DetailPanel } from './DetailPanel'
import { EvidenceBlock } from './EvidenceBlock'
import type { DetailItem } from './DetailPanel'
import { ResultCard } from './ResultCard'
import { WorkbenchEmptyState } from './WorkbenchEmptyState'
import type { ChatMessage, ChatMode, AIOpsEngine, WorkbenchMode } from '../../types/chat'
import { findSkillsByIds, formatSelectedSkillSummary } from '../../lib/utils'
import { isGoSEngine } from '../../hooks/useChat'
import { ENGINE_VIEW_MODEL } from '../../lib/engineViewModel'

interface Props {
  messages: ChatMessage[]
  streamingContent: string
  streamingThoughts: string[]
  thinkingSteps: ThinkingStep[]
  suggestions: Suggestion[]
  isLoading: boolean
  loadingEngine?: string | null
  mode: ChatMode
  workbenchMode: WorkbenchMode
  selectedSkillIds: string[]
  petEnabled: boolean
  aiOpsEngine: AIOpsEngine
  onSend: (query: string) => void
  onStartAIOps: (query: string) => void
  onStop: () => void
  onModeChange: (m: ChatMode) => void
  onTogglePet: () => void
  onClearSuggestions: () => void
}

const RESULT_STEP_IDS = new Set(['metrics', 'logs', 'knowledge', 'evidence', 'gos:hypothesis', 'gos:experts', 'gos:evidence', 'gos:confidence'])

function hasResultSteps(steps?: ThinkingStep[]): boolean {
  return steps?.some((step) => RESULT_STEP_IDS.has(step.id) || step.id.startsWith('tool:')) ?? false
}

const PLAN_STEP_PHASES: Record<string, string> = {
  engine: 'Plan',
  dispatch: 'Execute',
  evidence: 'Replan',
  reporter: 'Report',
}

function planStepToDetail(step: ThinkingStep): DetailItem | null {
  if (!step.detail && (!step.meta || step.meta.length === 0) && step.status !== 'error') return null
  return {
    id: step.id,
    type: step.status === 'error' ? 'error' : 'info',
    title: step.label,
    content: [step.detail, ...(step.meta || [])].filter(Boolean).join('\n\n') || `${step.label} 执行失败`,
    meta: step.status === 'error' ? '执行失败' : step.status === 'done' ? '已完成' : '执行中',
  }
}

function PlanExecutionTimeline({ steps, onOpenDetail }: { steps: ThinkingStep[]; onOpenDetail: (item: DetailItem) => void }) {
  const activeSteps = steps.filter((s) => s.status !== 'pending')
  if (activeSteps.length === 0) return null
  const view = ENGINE_VIEW_MODEL.plan_execute_replan

  return (
    <div className="ml-10 overflow-hidden rounded-[22px] rounded-bl-[6px] border border-sky-200/60 bg-white/65 backdrop-blur-xl dark:border-sky-500/15 dark:bg-slate-800/45">
      <div className="flex items-center justify-between border-b border-white/40 px-4 py-3 dark:border-white/5">
        <div className="flex items-center gap-2">
          <span className={`flex h-7 w-7 items-center justify-center rounded-lg ring-1 ${view.sidebar.icon}`}>
            <Route size={14} />
          </span>
          <div>
            <p className="text-xs font-semibold text-sky-700 dark:text-sky-300">Plan Timeline</p>
            <p className="text-[10px] text-zinc-400 dark:text-zinc-600">{view.trace}</p>
          </div>
        </div>
        <span className={`rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ${view.sidebar.flowActive}`}>
          {activeSteps.length} step
        </span>
      </div>

      <div className="px-4 py-3">
        {activeSteps.map((step, index) => {
          const detail = planStepToDetail(step)
          const clickable = Boolean(detail)
          const isLast = index === activeSteps.length - 1
          const phase = PLAN_STEP_PHASES[step.id] || 'Step'

          return (
            <div
              key={step.id}
              role={clickable ? 'button' : undefined}
              tabIndex={clickable ? 0 : undefined}
              onClick={clickable ? () => onOpenDetail(detail!) : undefined}
              onKeyDown={clickable ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpenDetail(detail!) } } : undefined}
              className={`group grid grid-cols-[22px_minmax(0,1fr)] gap-3 ${clickable ? 'cursor-pointer rounded-xl transition-colors hover:bg-white/55 dark:hover:bg-slate-700/30' : ''}`}
            >
              <div className="relative flex justify-center">
                {!isLast && <span className="absolute top-6 h-[calc(100%-10px)] w-px bg-sky-200/70 dark:bg-sky-500/20" />}
                <span className="relative mt-1 flex h-5 w-5 items-center justify-center rounded-full bg-white ring-1 ring-sky-200 dark:bg-slate-800 dark:ring-sky-500/20">
                  {step.status === 'active' ? (
                    <Loader2 size={12} className="animate-spin text-sky-500" />
                  ) : step.status === 'done' ? (
                    <CheckCircle2 size={13} className="text-emerald-500" />
                  ) : step.status === 'error' ? (
                    <CircleAlert size={13} className="text-rose-500" />
                  ) : (
                    <span className="h-1.5 w-1.5 rounded-full bg-zinc-300 dark:bg-zinc-600" />
                  )}
                </span>
              </div>

              <div className="min-w-0 pb-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">{step.label}</span>
                  <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${view.sidebar.flowActive}`}>
                    {phase}
                  </span>
                  {step.status === 'active' && <span className="text-[10px] font-medium text-sky-500">执行中...</span>}
                </div>
                {step.detail && <p className="mt-1 truncate text-[11px] text-zinc-500 dark:text-zinc-500">{step.detail}</p>}
                {step.meta && step.meta.length > 0 && (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {step.meta.slice(-2).map((item) => (
                      <span key={item} className="max-w-[220px] truncate rounded-md bg-white/55 px-2 py-0.5 text-[10px] text-zinc-500 dark:bg-slate-700/40 dark:text-zinc-400">
                        {item}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function AgentWorkbenchView({
  messages,
  streamingContent,
  streamingThoughts,
  thinkingSteps,
  suggestions,
  isLoading,
  loadingEngine,
  mode,
  workbenchMode,
  selectedSkillIds,
  petEnabled,
  aiOpsEngine,
  onSend,
  onStartAIOps,
  onStop,
  onModeChange,
  onTogglePet,
  onClearSuggestions,
}: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const selectedSkills = findSkillsByIds(selectedSkillIds)
  const [detailItem, setDetailItem] = useState<DetailItem | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const hasActiveConversation = messages.length > 0 || isLoading || streamingContent.length > 0

  useEffect(() => {
    if (messages.length === 0 && !streamingContent) return
    const scroller = scrollRef.current
    if (!scroller) return
    requestAnimationFrame(() => {
      scroller.scrollTo({ top: scroller.scrollHeight, behavior: 'smooth' })
    })
  }, [messages, streamingContent])

  const handleSuggestion = (query: string) => {
    onClearSuggestions()
    onSend(query)
  }

  const handleOpenDetail = useCallback((item: DetailItem) => {
    setDetailItem(item)
    setDetailOpen(true)
  }, [])

  const handleCloseDetail = useCallback(() => {
    setDetailOpen(false)
  }, [])

  const isGoS = isGoSEngine(loadingEngine)
  const isPlanAIOps = loadingEngine === 'plan_execute_replan'
  const activeEngineView = ENGINE_VIEW_MODEL[aiOpsEngine]

  return (
    <div className="flex h-full">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="shrink-0 border-b border-white/40 bg-white/30 px-4 py-2 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/20">
          <div className="mx-auto flex max-w-4xl items-center gap-3 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">
            <span className="inline-flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${isLoading ? 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.5)] animate-pulse' : 'bg-zinc-300 dark:bg-zinc-700'}`} />
              {workbenchMode === 'aiops' ? `${activeEngineView.label} 排障` : 'ReAct 问答'}
            </span>
            <span className="text-zinc-300 dark:text-zinc-700">·</span>
            <span>{workbenchMode === 'aiops' ? activeEngineView.trace : mode === 'quick' ? '快速' : '流式'}</span>
            {selectedSkills.length > 0 ? (
              <>
                <span className="text-zinc-300 dark:text-zinc-700">·</span>
                <span>{selectedSkills.length} 项能力</span>
                <span className="hidden sm:inline text-zinc-400 dark:text-zinc-600 truncate">
                  {selectedSkills.map(s => s.label).join('、')}
                </span>
              </>
            ) : (
              <>
                <span className="text-zinc-300 dark:text-zinc-700">·</span>
                <span>{formatSelectedSkillSummary(selectedSkillIds)}</span>
              </>
            )}

            <div className="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={onTogglePet}
                className="flex items-center gap-1 rounded-lg px-1.5 py-1 text-zinc-400 transition-colors hover:bg-white/50 hover:text-zinc-600 dark:text-zinc-600 dark:hover:bg-slate-700/50 dark:hover:text-zinc-400"
                aria-label={petEnabled ? '关闭运维助手' : '开启运维助手'}
                title={petEnabled ? '关闭运维助手' : '开启运维助手'}
              >
                {petEnabled ? <Bot size={14} /> : <BotOff size={14} />}
              </button>
              <button
                type="button"
                onClick={() => setDetailOpen((v) => !v)}
                className="flex items-center gap-1 rounded-lg px-1.5 py-1 text-zinc-400 transition-colors hover:bg-white/50 hover:text-zinc-600 dark:text-zinc-600 dark:hover:bg-slate-700/50 dark:hover:text-zinc-400"
                aria-label={detailOpen ? '关闭详情面板' : '打开详情面板'}
                title={detailOpen ? '关闭详情面板' : '打开详情面板'}
              >
                {detailOpen ? <PanelRightClose size={14} /> : <PanelRightOpen size={14} />}
              </button>
            </div>
          </div>
        </div>

        <div ref={scrollRef} className="relative min-h-0 flex-1 overflow-y-auto scrollbar-thin">
          <div className={`mx-auto flex min-h-full max-w-4xl flex-col gap-5 px-4 py-5 sm:py-6 ${hasActiveConversation ? 'justify-end' : ''}`}>
            {messages.length === 0 && !isLoading && (
              <WorkbenchEmptyState
                onSend={onSend}
                onStartAIOps={onStartAIOps}
                aiOpsEngine={aiOpsEngine}
              />
            )}

            <AnimatePresence initial={false}>
              {messages.map((msg, i) => {
                const isLastAssistant = msg.role === 'assistant' && i === messages.length - 1 && !isLoading
                const hasBlocks = hasResultSteps(msg.executionSteps)

                return (
                  <motion.div
                    key={msg.id}
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ type: 'spring', damping: 22, stiffness: 280 }}
                  >
                    <MessageBubble message={msg} onOpenDetail={handleOpenDetail} />

                    {isLastAssistant && hasBlocks && (
                      <div className="mt-3 ml-10">
                        <ResultCard
                          steps={msg.executionSteps!}
                          confidence={msg.confidence}
                          evidenceCount={msg.evidenceCount}
                          nextActions={msg.nextActions}
                          onRefine={() => onSend('请继续深入分析当前问题，补充更多证据和根因定位')}
                          onExport={() => {
                            const text = `# OpsCaptain 诊断报告\n\n${msg.content}\n\n---\n证据数: ${msg.executionSteps?.filter(s => s.status === 'done').length || 0}`
                            return navigator.clipboard.writeText(text)
                          }}
                        />
                      </div>
                    )}

                    {isLastAssistant && suggestions.length > 0 && (
                      <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }} className="mt-3 ml-10">
                        <SuggestionChips suggestions={suggestions} onSelect={handleSuggestion} />
                      </motion.div>
                    )}
                  </motion.div>
                )
              })}
            </AnimatePresence>

            {isLoading && (
              <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} className="space-y-3">
                {isPlanAIOps ? (
                  <PlanExecutionTimeline steps={thinkingSteps} onOpenDetail={handleOpenDetail} />
                ) : !isGoS && thinkingSteps.filter((s) => s.status !== 'pending').length > 0 ? (
                  <div className="ml-10 space-y-2">
                    {thinkingSteps
                      .filter((s) => s.status !== 'pending')
                      .map((step) => (
                        <EvidenceBlock
                          key={step.id}
                          step={step}
                          onOpenDetail={handleOpenDetail}
                        />
                      ))}
                  </div>
                ) : null}

                {isGoS ? (
                  <GoSBeliefProgress steps={thinkingSteps} content={streamingContent} />
                ) : streamingContent ? (
                  <div className="rounded-[22px] rounded-bl-[6px] border border-white/60 bg-white/70 px-4 py-3 backdrop-blur-xl dark:border-white/10 dark:bg-slate-800/50">
                    <StreamingText content={streamingContent} />
                  </div>
                ) : thinkingSteps.filter((s) => s.status !== 'pending').length === 0 ? (
                  <div className="flex items-center gap-3 rounded-xl border border-white/40 bg-white/40 px-4 py-3 backdrop-blur-sm dark:border-white/5 dark:bg-slate-800/30">
                    <span className="h-2 w-2 rounded-full bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.5)] animate-pulse" />
                    <span className="text-xs text-sky-600/70 dark:text-sky-400/70">正在启动诊断...</span>
                  </div>
                ) : null}
              </motion.div>
            )}

            <div ref={bottomRef} />
          </div>
        </div>

        <div className="shrink-0 border-t border-white/40 bg-white/40 backdrop-blur-xl dark:border-white/5 dark:bg-slate-900/30">
          <div className="mx-auto flex max-w-5xl items-end gap-2 px-4 py-3 sm:gap-4 sm:px-5">
            <div className="min-w-0 flex-1">
              <ChatInput
                onSend={onSend}
                onStop={onStop}
                isLoading={isLoading}
                mode={mode}
                workbenchMode={workbenchMode}
                aiOpsEngine={aiOpsEngine}
                selectedSkillIds={selectedSkillIds}
                onModeChange={onModeChange}
                embedded
              />
            </div>
          </div>
        </div>
      </div>

      <AnimatePresence>
        {detailOpen && (
          <DetailPanel item={detailItem} onClose={handleCloseDetail} />
        )}
      </AnimatePresence>
    </div>
  )
}
