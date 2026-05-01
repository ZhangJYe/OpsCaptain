import { useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, Waves } from 'lucide-react'
import { MessageBubble } from './MessageBubble'
import { StreamingText } from './StreamingText'
import { ChatInput } from './ChatInput'
import type { ChatMessage, ChatMode } from '../../types/chat'
import { findSkillsByIds, formatSelectedSkillSummary } from '../../lib/utils'

interface Props {
  messages: ChatMessage[]
  streamingContent: string
  streamingThoughts: string[]
  isLoading: boolean
  mode: ChatMode
  selectedSkillIds: string[]
  onSend: (query: string) => void
  onStop: () => void
  onModeChange: (m: ChatMode) => void
}

export function ChatView({
  messages,
  streamingContent,
  streamingThoughts,
  isLoading,
  mode,
  selectedSkillIds,
  onSend,
  onStop,
  onModeChange,
}: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)
  const selectedSkills = findSkillsByIds(selectedSkillIds)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  return (
    <div className="flex flex-col h-full">
      <div className="border-b border-zinc-200/80 bg-white/72 px-4 py-2 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/42">
        <div className="mx-auto flex max-w-4xl items-center gap-3 text-xs text-zinc-500 dark:text-zinc-500">
          <span className="inline-flex items-center gap-1">
            {mode === 'quick' ? <Activity size={12} className="text-accent" /> : <Waves size={12} className="text-accent" />}
            {mode === 'quick' ? '快速回答' : '流式输出'}
          </span>
          {selectedSkills.length > 0 ? (
            <>
              <span className="text-zinc-300 dark:text-zinc-700">|</span>
              <span>{selectedSkills.length} 个能力已启用</span>
              <span className="hidden sm:inline text-zinc-400 dark:text-zinc-600">
                — {selectedSkills.map(s => s.label).join('、')}
              </span>
            </>
          ) : (
            <>
              <span className="text-zinc-300 dark:text-zinc-700">|</span>
              <span>{formatSelectedSkillSummary(selectedSkillIds)}</span>
            </>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin px-4 py-6">
        <div className="mx-auto max-w-4xl space-y-6">
          <AnimatePresence initial={false}>
            {messages.map((msg) => (
              <motion.div
                key={msg.id}
                initial={{ opacity: 0, y: 16, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                transition={{ type: 'spring', damping: 20, stiffness: 200 }}
              >
                <MessageBubble message={msg} />
              </motion.div>
            ))}
          </AnimatePresence>

          {isLoading && (
            <motion.div
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              className="flex items-start gap-3"
            >
              <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl border border-accent/20 bg-accent/10">
                <span className="text-xs font-bold text-accent">AI</span>
              </div>
              <div className="flex-1 rounded-3xl border border-zinc-200/80 bg-white/85 px-5 py-4 dark:border-zinc-800/80 dark:bg-zinc-900/62">
                <div className="mb-3 flex items-center gap-2 text-[11px] text-zinc-500 dark:text-zinc-500">
                  <span className="font-medium text-zinc-800 dark:text-zinc-300">OpsCaption</span>
                  <span>正在整理证据与结论</span>
                </div>
                {streamingThoughts.length > 0 ? (
                  <div className="mb-3 space-y-2 rounded-2xl border border-zinc-200/80 bg-zinc-50/90 px-4 py-3 dark:border-zinc-800/80 dark:bg-zinc-950/70">
                    {streamingThoughts.map((thought) => (
                      <div key={thought} className="text-xs leading-5 text-zinc-600 dark:text-zinc-400">
                        {thought}
                      </div>
                    ))}
                  </div>
                ) : null}
                {streamingContent ? (
                  <StreamingText content={streamingContent} />
                ) : (
                  <div className="flex items-center gap-1.5 py-3">
                    <span className="w-2 h-2 rounded-full bg-accent animate-pulse-dot" />
                    <span className="w-2 h-2 rounded-full bg-accent animate-pulse-dot" style={{ animationDelay: '0.2s' }} />
                    <span className="w-2 h-2 rounded-full bg-accent animate-pulse-dot" style={{ animationDelay: '0.4s' }} />
                  </div>
                )}
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
