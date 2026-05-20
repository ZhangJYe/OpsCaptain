import { useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, Waves, Bot, BotOff } from 'lucide-react'
import { MessageBubble } from './MessageBubble'
import { StreamingText } from './StreamingText'
import { ChatInput } from './ChatInput'
import { GosReportCard } from './GosReportCard'
import { PetCharacter } from '../pet/PetCharacter'
import type { PetMood } from '../pet/PetCharacter'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { SuggestionChips } from '../agent/SuggestionChips'
import type { Suggestion } from '../agent/SuggestionChips'
import type { ChatMessage, ChatMode } from '../../types/chat'
import { findSkillsByIds, formatSelectedSkillSummary } from '../../lib/utils'
import { isGoSEngine } from '../../hooks/useChat'

function resolvePetMood(steps: ThinkingStep[], isStreaming: boolean, isGoS: boolean): PetMood {
  if (isGoS && isStreaming) return 'gos'
  if (steps.some((s) => s.status === 'error')) return 'error'
  if (isStreaming || steps.some((s) => s.status === 'active')) return 'thinking'
  if (steps.length > 0 && steps.every((s) => s.status === 'done')) return 'done'
  return 'idle'
}

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

export function ChatView({
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
  const scrollRef = useRef<HTMLDivElement>(null)
  const selectedSkills = findSkillsByIds(selectedSkillIds)

  useEffect(() => {
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

  return (
    <div className="flex flex-col h-full">
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
          <button
            type="button"
            onClick={onTogglePet}
            className="ml-auto flex items-center gap-1 rounded-md px-1.5 py-1 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-400"
            aria-label={petEnabled ? '关闭运维助手' : '开启运维助手'}
            title={petEnabled ? '关闭运维助手' : '开启运维助手'}
          >
            {petEnabled ? <Bot size={14} /> : <BotOff size={14} />}
            <span className="hidden sm:inline">{petEnabled ? '助手' : '关闭'}</span>
          </button>
        </div>
      </div>

      <div ref={scrollRef} className="relative flex-1 overflow-y-auto scrollbar-thin">
        <div className="mx-auto max-w-4xl px-4 py-8 space-y-6">

          <AnimatePresence initial={false}>
            {messages.map((msg, i) => (
              <motion.div
                key={msg.id}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ type: 'spring', damping: 24, stiffness: 260 }}
              >
                <MessageBubble message={msg} />
                {msg.role === 'assistant' && i === messages.length - 1 && !isLoading && suggestions.length > 0 && (
                  <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }} className="mt-3 ml-11">
                    <SuggestionChips suggestions={suggestions} onSelect={handleSuggestion} />
                  </motion.div>
                )}
              </motion.div>
            ))}
          </AnimatePresence>

          {/* Streaming bubble — thinking collapse embedded inside */}
          {isLoading && (
            <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} className="flex items-start gap-3">
              <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-accent/20 bg-accent/10 text-xs font-semibold text-accent">
                OC
              </div>
              <div className="min-w-0 flex-1 max-w-[85%]">
                <div className="mb-1.5 flex items-center gap-2">
                  <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">OpsCaption</span>
                  <span className="text-[10px] text-zinc-400 dark:text-zinc-600">
                    {streamingContent ? '生成中' : '处理中'}
                  </span>
                </div>
                {isGoSEngine(loadingEngine) ? (
                  <GosReportCard
                    steps={thinkingSteps}
                    content={streamingContent}
                    isStreaming
                  />
                ) : (
                  <div className="rounded-2xl border border-zinc-200/80 bg-white/95 px-4 py-3 shadow-sm shadow-zinc-900/[0.03] dark:border-zinc-800/60 dark:bg-zinc-900/80">
                    <ThinkingCollapse steps={thinkingSteps} isStreaming />
                    {streamingContent ? (
                      <StreamingText content={streamingContent} />
                    ) : (
                      <div className="flex items-center gap-1.5 py-2">
                        <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot" />
                        <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot [animation-delay:0.2s]" />
                        <span className="w-2 h-2 rounded-full bg-accent/60 animate-pulse-dot [animation-delay:0.4s]" />
                      </div>
                    )}
                  </div>
                )}
              </div>
            </motion.div>
          )}

          {petEnabled && (messages.length > 0 || isLoading) && (
            <div className="pointer-events-none absolute bottom-4 right-4 z-10 select-none">
              <div className="pointer-events-auto">
                <PetCharacter
                  mood={resolvePetMood(thinkingSteps, isLoading, isGoSEngine(loadingEngine))}
                  size={48}
                />
              </div>
            </div>
          )}

          <div ref={bottomRef} />
        </div>
      </div>

      <ChatInput
        onSend={onSend}
        onStop={onStop}
        isLoading={isLoading}
        mode={mode}
        selectedSkillIds={selectedSkillIds}
        onModeChange={onModeChange}
      />
    </div>
  )
}
