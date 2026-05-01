import { useState, useRef, useEffect } from 'react'
import { GitBranch, Send, Square, Zap } from 'lucide-react'
import type { ChatMode } from '../../types/chat'
import { formatSelectedSkillSummary } from '../../lib/utils'

interface Props {
  onSend: (query: string) => void
  onStop: () => void
  isLoading: boolean
  mode: ChatMode
  selectedSkillIds: string[]
  onModeChange: (m: ChatMode) => void
}

const modeOptions: { id: ChatMode; label: string; icon: typeof Zap }[] = [
  { id: 'quick', label: '快速', icon: Zap },
  { id: 'stream', label: '流式', icon: GitBranch },
]

export function ChatInput({ onSend, onStop, isLoading, mode, selectedSkillIds, onModeChange }: Props) {
  const [input, setInput] = useState('')
  const [isFocused, setIsFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const handleSubmit = () => {
    if (!input.trim() || isLoading) return
    onSend(input.trim())
    setInput('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div className="shrink-0 border-t border-zinc-200/80 bg-white/88 px-4 py-4 backdrop-blur-xl dark:border-zinc-900/80 dark:bg-zinc-950/80">
      <div className="mx-auto max-w-4xl">
        <div
          className={`rounded-2xl border transition-all duration-300 ${
            isFocused
              ? 'border-accent/30 shadow-[0_0_0_3px_rgba(59,130,246,0.08)] dark:shadow-[0_0_0_3px_rgba(59,130,246,0.06)]'
              : 'border-zinc-200/80 shadow-sm dark:border-zinc-800/60'
          } bg-white/90 dark:bg-zinc-900/70`}
        >
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onFocus={() => setIsFocused(true)}
            onBlur={() => setIsFocused(false)}
            placeholder="描述告警、日志或系统现象..."
            rows={1}
            className="min-h-[44px] w-full resize-none bg-transparent px-4 py-3 text-sm leading-7 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
          />

          <div className="flex items-center justify-between gap-3 border-t border-zinc-100 px-3 py-2.5 dark:border-zinc-800">
            <div className="flex items-center gap-3">
              {/* Mode toggle */}
              <div className="inline-flex rounded-lg bg-zinc-100 p-0.5 dark:bg-zinc-800">
                {modeOptions.map((option) => (
                  <button
                    key={option.id}
                    onClick={() => onModeChange(option.id)}
                    className={`flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-all duration-200 ${
                      option.id === mode
                        ? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-700 dark:text-white'
                        : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
                    }`}
                  >
                    <option.icon size={13} />
                    <span>{option.label}</span>
                  </button>
                ))}
              </div>

              {/* Skill summary */}
              <span className="hidden text-[11px] text-zinc-400 dark:text-zinc-600 sm:inline truncate max-w-[200px]">
                {formatSelectedSkillSummary(selectedSkillIds)}
              </span>
            </div>

            <div className="flex items-center gap-2">
              <span className="hidden text-[10px] text-zinc-400 dark:text-zinc-600 lg:inline">
                Enter 发送 · Shift+Enter 换行
              </span>
              <button
                onClick={isLoading ? onStop : handleSubmit}
                className={`inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2 text-sm font-medium transition-all duration-200 ${
                  isLoading
                    ? 'bg-red-500/10 text-red-400 hover:bg-red-500/20'
                    : input.trim()
                      ? 'bg-accent text-white shadow-sm hover:brightness-110 active:scale-[0.97]'
                      : 'cursor-not-allowed bg-zinc-100 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-600'
                }`}
              >
                {isLoading ? (
                  <>
                    <Square size={14} />
                    停止
                  </>
                ) : (
                  <>
                    <Send size={14} />
                    发送
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
