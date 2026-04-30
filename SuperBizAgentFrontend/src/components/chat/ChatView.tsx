import { useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Activity, Database, Waves } from 'lucide-react'
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
      <div className="border-b border-zinc-900/80 bg-zinc-950/42 px-4 py-3 backdrop-blur-xl">
        <div className="mx-auto flex max-w-4xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="text-[11px] uppercase tracking-[0.22em] text-zinc-600">Current Session</div>
            <div className="mt-1 text-sm text-zinc-300">
              围绕 metrics、logs、knowledge 组织本轮诊断输出
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <span className="inline-flex items-center gap-1 rounded-full border border-zinc-800/80 bg-zinc-900/70 px-3 py-1.5 text-zinc-400">
              <Database size={12} className="text-accent" />
              上下文已装配
            </span>
            <span className="inline-flex items-center gap-1 rounded-full border border-zinc-800/80 bg-zinc-900/70 px-3 py-1.5 text-zinc-400">
              {mode === 'quick' ? <Activity size={12} className="text-accent" /> : <Waves size={12} className="text-accent" />}
              {mode === 'quick' ? '快速回答' : '流式输出'}
            </span>
          </div>
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
              <div className="flex-1 rounded-3xl border border-zinc-800/80 bg-zinc-900/62 px-5 py-4">
                <div className="mb-3 flex items-center gap-2 text-[11px] text-zinc-500">
                  <span className="font-medium text-zinc-300">OpsCaption</span>
                  <span>正在整理证据与结论</span>
                </div>
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

      <ChatInput onSend={onSend} onStop={onStop} isLoading={isLoading} mode={mode} onModeChange={onModeChange} />
    </div>
  )
}
