import { useState, useRef, useEffect } from 'react'
import { GitBranch, Send, Square, Zap } from 'lucide-react'
import type { ChatMode } from '../../types/chat'

interface Props {
  onSend: (query: string) => void
  onStop: () => void
  isLoading: boolean
  mode: ChatMode
  onModeChange: (m: ChatMode) => void
}

export function ChatInput({ onSend, onStop, isLoading, mode, onModeChange }: Props) {
  const [input, setInput] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const modeOptions: { id: ChatMode; label: string; description: string; icon: typeof Zap }[] = [
    { id: 'quick', label: '快速', description: '一次返回完整答案', icon: Zap },
    { id: 'stream', label: '流式', description: '边生成边展示', icon: GitBranch },
  ]

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 200) + 'px'
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
    <div className="border-t border-zinc-900/80 bg-zinc-950/80 px-4 py-4 backdrop-blur-xl">
      <div className="mx-auto max-w-4xl">
        <div className="rounded-[28px] border border-zinc-800/80 bg-zinc-900/70 p-3 shadow-[0_20px_60px_rgba(0,0,0,0.22)]">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="描述告警、日志或系统现象..."
            rows={1}
            className="min-h-[84px] w-full resize-none bg-transparent px-3 py-3 text-sm leading-7 text-zinc-100 outline-none placeholder:text-zinc-500"
          />

          <div className="mt-3 flex flex-col gap-3 border-t border-zinc-800/80 px-1 pt-3 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex flex-col gap-2 lg:flex-row lg:items-center lg:gap-4">
              <div className="inline-flex w-full rounded-2xl border border-zinc-800/80 bg-zinc-950/70 p-1 lg:w-auto">
                {modeOptions.map((option) => (
                  <button
                    key={option.id}
                    onClick={() => onModeChange(option.id)}
                    className={`flex min-w-[112px] flex-1 items-center justify-center gap-2 rounded-2xl px-3 py-2 text-xs font-medium transition-colors ${
                      option.id === mode
                        ? 'bg-accent/16 text-accent'
                        : 'text-zinc-500 hover:text-zinc-300'
                    }`}
                  >
                    <option.icon size={14} />
                    <span>{option.label}</span>
                  </button>
                ))}
              </div>
              <div className="text-[11px] text-zinc-500">
                Enter 发送，Shift + Enter 换行
              </div>
            </div>

            <button
              onClick={isLoading ? onStop : handleSubmit}
              className={`inline-flex min-w-[108px] items-center justify-center gap-2 rounded-2xl px-4 py-3 text-sm font-medium transition-all duration-200 ${
                isLoading
                  ? 'bg-red-500/16 text-red-300 hover:bg-red-500/24'
                  : input.trim()
                    ? 'bg-accent text-white hover:brightness-110'
                    : 'bg-zinc-800/90 text-zinc-600'
              }`}
            >
              {isLoading ? (
                <>
                  <Square size={16} />
                  停止
                </>
              ) : (
                <>
                  <Send size={16} />
                  发送
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
