import { useRef, useEffect, useState, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Bot, BotOff, PanelRightOpen, PanelRightClose } from 'lucide-react'
import { MessageBubble } from '../chat/MessageBubble'
import { StreamingText } from '../chat/StreamingText'
import { ChatInput } from '../chat/ChatInput'
import { GosReportCard } from '../chat/GosReportCard'
import { SuggestionChips } from '../agent/SuggestionChips'
import type { Suggestion } from '../agent/SuggestionChips'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { CompanionBar } from './CompanionBar'
import { DetailPanel } from './DetailPanel'
import { EvidenceBlock } from './EvidenceBlock'
import type { DetailItem } from './DetailPanel'
import { ResultCard } from './ResultCard'
import { WorkbenchEmptyState } from './WorkbenchEmptyState'
import type { ChatMessage, ChatMode, AIOpsEngine } from '../../types/chat'
import { findSkillsByIds, formatSelectedSkillSummary } from '../../lib/utils'
import { isGoSEngine } from '../../hooks/useChat'

interface Props {
  messages: ChatMessage[]
  streamingContent: string
  streamingThoughts: string[]
  thinkingSteps: ThinkingStep[]
  suggestions: Suggestion[]
  isLoading: boolean
  loadingEngine?: string | null
  mode: ChatMode
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

export function AgentWorkbenchView({
  messages,
  streamingContent,
  streamingThoughts,
  thinkingSteps,
  suggestions,
  isLoading,
  loadingEngine,
  mode,
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
  const selectedSkills = findSkillsByIds(selectedSkillIds)
  const [detailItem, setDetailItem] = useState<DetailItem | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  useEffect(() => {
    if (messages.length === 0 && !streamingContent) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
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

  return (
    <div className="flex h-full">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="shrink-0 border-b border-white/40 bg-white/30 px-4 py-2 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/20">
          <div className="mx-auto flex max-w-4xl items-center gap-3 text-[11px] font-medium text-zinc-500 dark:text-zinc-500">
            <span className="inline-flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${isLoading ? 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.5)] animate-pulse' : 'bg-zinc-300 dark:bg-zinc-700'}`} />
              {mode === 'quick' ? '快速回答' : '流式输出'}
            </span>
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

        <div className="relative flex-1 overflow-y-auto scrollbar-thin">
          <div className="mx-auto max-w-4xl px-4 py-6 space-y-5">

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
                const hasBlocks = msg.executionSteps && msg.executionSteps.length > 0

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
                {thinkingSteps.filter((s) => s.status !== 'pending').length > 0 && (
                  <div className="space-y-2">
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
                )}

                {isGoS ? (
                  <GosReportCard steps={thinkingSteps} content={streamingContent} isStreaming />
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

        <div className="shrink-0 border-t border-white/40 bg-white/30 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/20">
          <div className="mx-auto flex max-w-4xl items-end gap-3 px-4 py-3">
            {petEnabled && (
              <CompanionBar
                steps={thinkingSteps}
                isStreaming={isLoading}
                isGoS={isGoS}
              />
            )}
            <div className="min-w-0 flex-1">
              <ChatInput
                onSend={onSend}
                onStop={onStop}
                isLoading={isLoading}
                mode={mode}
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
