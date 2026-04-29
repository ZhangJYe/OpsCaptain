import { useState, useRef, useEffect } from 'react'
import { Send, Square, ChevronDown } from 'lucide-react'
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
  const [modeOpen, setModeOpen] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

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
    <div className="border-t border-zinc-800 px-4 py-3">
      <div className="max-w-3xl mx-auto">
        <div className="glass rounded-2xl p-2 flex items-end gap-2">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="描述告警、日志或系统现象..."
            rows={1}
            className="flex-1 bg-transparent resize-none outline-none text-sm px-3 py-2 max-h-[200px] placeholder:text-zinc-500"
          />

          <div className="flex items-center gap-1 shrink-0">
            <div className="relative">
              <button
                onClick={() => setModeOpen(!modeOpen)}
                className="p-2 rounded-lg hover:bg-zinc-800 transition-colors text-xs text-zinc-400"
              >
                <ChevronDown size={14} />
              </button>
              {modeOpen && (
                <div className="absolute bottom-full right-0 mb-2 glass rounded-xl p-1 min-w-[140px] z-50">
                  {(['quick', 'stream'] as ChatMode[]).map((m) => (
                    <button
                      key={m}
                      onClick={() => {
                        onModeChange(m)
                        setModeOpen(false)
                      }}
                      className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                        m === mode ? 'bg-accent/20 text-accent' : 'hover:bg-zinc-800'
                      }`}
                    >
                      <div className="font-medium">{m === 'quick' ? '快速回答' : '流式输出'}</div>
                      <div className="text-xs text-zinc-500">{m === 'quick' ? '一次性返回' : '边生成边展示'}</div>
                    </button>
                  ))}
                </div>
              )}
            </div>

            <button
              onClick={isLoading ? onStop : handleSubmit}
              className={`p-2.5 rounded-xl transition-all duration-200 ${
                isLoading
                  ? 'bg-red-500/20 text-red-400 hover:bg-red-500/30'
                  : input.trim()
                    ? 'bg-accent text-white hover:opacity-90'
                    : 'bg-zinc-800 text-zinc-600'
              }`}
            >
              {isLoading ? <Square size={18} /> : <Send size={18} />}
            </button>
          </div>
        </div>
        <p className="text-xs text-zinc-600 text-center mt-2">Enter 发送 · Shift+Enter 换行</p>
      </div>
    </div>
  )
}
