import { useRef, useEffect, useState, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, Waves, Bot, BotOff, PanelRightOpen, PanelRightClose } from 'lucide-react'
import { MessageBubble } from '../chat/MessageBubble'
import { StreamingText } from '../chat/StreamingText'
import { ChatInput } from '../chat/ChatInput'
import { GosReportCard } from '../chat/GosReportCard'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { SuggestionChips } from '../agent/SuggestionChips'
import type { Suggestion } from '../agent/SuggestionChips'
import { CompanionBar } from './CompanionBar'
import { DetailPanel } from './DetailPanel'
import type { DetailItem } from './DetailPanel'
import { EvidenceBlock } from './EvidenceBlock'
import type { ChatMessage, ChatMode } from '../../types/chat'
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
  onSend: (query: string) => void
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
  onSend,
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
      {/* Center: stream + input */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Header bar */}
        <div className="shrink-0 border-b border-zinc-200/80 bg-white/70 px-4 py-2 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/40">
          <div className="mx-auto flex max-w-4xl items-center gap-3 text-xs text-zinc-500 dark:text-zinc-500">
            <span className="inline-flex items-center gap-1.5">
              {mode === 'quick' ? <Activity size={12} className="text-accent" /> : <Waves size={12} className="text-accent" />}
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
                className="flex items-center gap-1 rounded-md px-1.5 py-1 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-400"
                aria-label={petEnabled ? '关闭运维助手' : '开启运维助手'}
                title={petEnabled ? '关闭运维助手' : '开启运维助手'}
              >
                {petEnabled ? <Bot size={14} /> : <BotOff size={14} />}
              </button>
              <button
                type="button"
                onClick={() => setDetailOpen((v) => !v)}
                className="flex items-center gap-1 rounded-md px-1.5 py-1 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-400"
                aria-label={detailOpen ? '关闭详情面板' : '打开详情面板'}
                title={detailOpen ? '关闭详情面板' : '打开详情面板'}
              >
                {detailOpen ? <PanelRightClose size={14} /> : <PanelRightOpen size={14} />}
              </button>
            </div>
          </div>
        </div>

        {/* Evidence stream */}
        <div className="relative flex-1 overflow-y-auto scrollbar-thin">
          <div className="mx-auto max-w-4xl px-4 py-8 space-y-6">

            <AnimatePresence initial={false}>
              {messages.map((msg, i) => {
                const isLastAssistant = msg.role === 'assistant' && i === messages.length - 1 && !isLoading
                const hasBlocks = msg.executionSteps && msg.executionSteps.length > 0

                return (
                  <motion.div
                    key={msg.id}
                    initial={{ opacity: 0, y: 12 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ type: 'spring', damping: 24, stiffness: 260 }}
                  >
                    {/* Evidence blocks above the last assistant message */}
                    {isLastAssistant && hasBlocks && (
                      <div className="mb-3 space-y-2">
                        {msg.executionSteps!.map((step) => (
                          <EvidenceBlock
                            key={step.id}
                            step={step}
                            onOpenDetail={handleOpenDetail}
                          />
                        ))}
                      </div>
                    )}

                    <MessageBubble message={msg} onOpenDetail={handleOpenDetail} hideSteps={isLastAssistant && hasBlocks} />

                    {isLastAssistant && suggestions.length > 0 && (
                      <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }} className="mt-3 ml-11">
                        <SuggestionChips suggestions={suggestions} onSelect={handleSuggestion} />
                      </motion.div>
                    )}
                  </motion.div>
                )
              })}
            </AnimatePresence>

            {/* Streaming: evidence blocks + output */}
            {isLoading && (
              <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} className="space-y-3">
                {/* Evidence blocks */}
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

                {/* Streaming output */}
                {isGoS ? (
                  <GosReportCard steps={thinkingSteps} content={streamingContent} isStreaming />
                ) : streamingContent ? (
                  <div className="rounded-2xl border border-zinc-200/80 bg-white/95 px-4 py-3 shadow-sm shadow-zinc-900/[0.03] dark:border-zinc-800/60 dark:bg-zinc-900/80">
                    <StreamingText content={streamingContent} />
                  </div>
                ) : thinkingSteps.filter((s) => s.status !== 'pending').length === 0 ? (
                  <div className="flex items-center gap-3 rounded-xl border border-zinc-200/80 bg-white/80 px-4 py-3 dark:border-zinc-800/60 dark:bg-zinc-900/60">
                    <div className="flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot" />
                      <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot [animation-delay:0.2s]" />
                      <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot [animation-delay:0.4s]" />
                    </div>
                    <span className="text-xs text-zinc-400">正在启动诊断...</span>
                  </div>
                ) : null}
              </motion.div>
            )}

            <div ref={bottomRef} />
          </div>
        </div>

        {/* Companion + Input area */}
        <div className="shrink-0 border-t border-zinc-200/80 bg-white/88 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/80">
          <div className="mx-auto flex max-w-4xl items-end gap-3 px-4 py-3">
            {petEnabled && (messages.length > 0 || isLoading) && (
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

      {/* Right detail panel */}
      <AnimatePresence>
        {detailOpen && (
          <DetailPanel item={detailItem} onClose={handleCloseDetail} />
        )}
      </AnimatePresence>
    </div>
  )
}
