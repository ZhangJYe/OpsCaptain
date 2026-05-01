import { useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, Waves } from 'lucide-react'
import { MessageBubble } from './MessageBubble'
import { StreamingText } from './StreamingText'
import { ChatInput } from './ChatInput'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import { SuggestionChips } from '../agent/SuggestionChips'
import type { Suggestion } from '../agent/SuggestionChips'
import type { ChatMessage, ChatMode } from '../../types/chat'
import { findSkillsByIds, formatSelectedSkillSummary } from '../../lib/utils'

interface Props {
  messages: ChatMessage[]
  streamingContent: string
  streamingThoughts: string[]
  thinkingSteps: ThinkingStep[]
  suggestions: Suggestion[]
  isLoading: boolean
  mode: ChatMode
  selectedSkillIds: string[]
  onSend: (query: string) => void
  onStop: () => void
  onModeChange: (m: ChatMode) => void
  onClearSuggestions: () => void
}

export function ChatView({
  messages,
  streamingContent,
  streamingThoughts,
  thinkingSteps,
  suggestions,
  isLoading,
  mode,
  selectedSkillIds,
  onSend,
  onStop,
  onModeChange,
  onClearSuggestions,
}: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)
  const selectedSkills = findSkillsByIds(selectedSkillIds)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const handleSuggestion = (query: string) => {
    onClearSuggestions()
    onSend(query)
  }

  return (
    <div className="flex flex-col h-full">
      <div className="shrink-0 border-b border-zinc-200/80 bg-white/72 px-4 py-2 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/42">
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
        </div>
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin">
        <div className="mx-auto max-w-4xl px-4 py-6 space-y-5">

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
              <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent/10 text-xs font-semibold text-accent ring-1 ring-inset ring-accent/20">
                AI
              </div>
              <div className="min-w-0 flex-1 max-w-[85%]">
                <div className="mb-1.5 flex items-center gap-2">
                  <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">OpsCaption</span>
                  <span className="text-[10px] text-zinc-400 dark:text-zinc-600">
                    {streamingContent ? '生成中' : '处理中'}
                  </span>
                </div>
                <div className="rounded-2xl border border-zinc-200/80 bg-white/90 px-4 py-3 shadow-sm dark:border-zinc-800/60 dark:bg-zinc-900/70">

                  {/* Thinking collapse — like DeepSeek */}
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
              </div>
            </motion.div>
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
