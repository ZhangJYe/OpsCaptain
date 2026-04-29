import { useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { MessageBubble } from './MessageBubble'
import { StreamingText } from './StreamingText'
import { ChatInput } from './ChatInput'
import type { ChatMessage, ChatMode } from '../../types/chat'

interface Props {
  messages: ChatMessage[]
  streamingContent: string
  isLoading: boolean
  mode: ChatMode
  onSend: (query: string) => void
  onStop: () => void
  onModeChange: (m: ChatMode) => void
}

export function ChatView({ messages, streamingContent, isLoading, mode, onSend, onStop, onModeChange }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto scrollbar-thin px-4 py-6">
        <div className="max-w-3xl mx-auto space-y-6">
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
              <div className="w-8 h-8 rounded-lg bg-accent/20 flex items-center justify-center shrink-0 mt-1">
                <span className="text-xs text-accent font-bold">AI</span>
              </div>
              <div className="flex-1">
                {streamingContent ? (
                  <StreamingText content={streamingContent} />
                ) : (
                  <div className="flex items-center gap-1.5 py-4">
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

      <ChatInput onSend={onSend} onStop={onStop} isLoading={isLoading} mode={mode} onModeChange={onModeChange} />
    </div>
  )
}
