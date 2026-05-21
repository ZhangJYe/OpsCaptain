import { useState, useRef, useEffect } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { GitBranch, Send, Square, Zap, Paperclip, X, Loader2, FileIcon } from 'lucide-react'
import type { AIOpsEngine, ChatMode, WorkbenchMode } from '../../types/chat'
import { formatSelectedSkillSummary, formatFileSize } from '../../lib/utils'
import { useFileUpload } from '../../hooks/useFileUpload'
import { getEngineViewModel } from '../../lib/engineViewModel'

interface Props {
  onSend: (query: string) => void
  onStop: () => void
  isLoading: boolean
  mode: ChatMode
  workbenchMode: WorkbenchMode
  aiOpsEngine: AIOpsEngine
  selectedSkillIds: string[]
  onModeChange: (m: ChatMode) => void
  embedded?: boolean
}

const modeOptions: { id: ChatMode; label: string; icon: typeof Zap }[] = [
  { id: 'quick', label: '快速', icon: Zap },
  { id: 'stream', label: '流式', icon: GitBranch },
]

function buildQueryWithFiles(query: string, fileNames: string[]): string {
  if (fileNames.length === 0) return query
  const refs = fileNames.map((n) => `[已上传: ${n}]`).join('\n')
  return `${refs}\n\n${query}`
}

export function ChatInput({ onSend, onStop, isLoading, mode, workbenchMode, aiOpsEngine, selectedSkillIds, onModeChange, embedded }: Props) {
  const [input, setInput] = useState('')
  const [isFocused, setIsFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const { files, readyFiles, isUploading, uploadError, removeFile, clearFiles, inputId, handleChange, accept, multiple } = useFileUpload()
  const isAIOps = workbenchMode === 'aiops'
  const engineView = getEngineViewModel(aiOpsEngine)
  const submitLabel = isAIOps ? `启动 ${engineView.label}` : '发送'
  const placeholder = isAIOps
    ? `描述告警、日志或系统现象，使用 ${engineView.label} 排障...`
    : '输入问题，使用 ReAct 问答...'
  const contextLabel = isAIOps ? `AIOps · ${engineView.badge}` : formatSelectedSkillSummary(selectedSkillIds)
  const trimmedInput = input.trim()
  const hasDraft = trimmedInput.length > 0 || readyFiles.length > 0
  const aiOpsReady = !isAIOps || readyFiles.length > 0 || Array.from(trimmedInput).length >= 6
  const canSend = hasDraft && aiOpsReady && !isLoading
  const sendHint = isAIOps && hasDraft && !aiOpsReady
    ? '补充服务、告警或日志线索'
    : 'Enter 发送 · Shift+Enter 换行'

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const handleSubmit = () => {
    if (!canSend) return
    const names = readyFiles.map((f) => f.name)
    const query = buildQueryWithFiles(trimmedInput, names)
    onSend(query || '请分析上传的文件')
    setInput('')
    clearFiles()
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div className={embedded ? 'shrink-0' : 'shrink-0 border-t border-white/40 bg-white/30 px-4 py-4 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/30'}>
      <input type="file" id={inputId} onChange={handleChange} accept={accept} multiple={multiple} className="hidden" />

      <div className={embedded ? '' : 'mx-auto max-w-4xl'}>
        <div className={`relative rounded-[22px] rounded-bl-[6px] transition-shadow duration-300 ${isFocused ? 'shadow-lg shadow-sky-500/10 dark:shadow-none' : 'shadow-md shadow-zinc-900/5 dark:shadow-none'}`}>
          {isFocused && <div aria-hidden="true" className="glow-frame rounded-[22px] rounded-bl-[6px]" />}

          <div className={`relative rounded-[22px] rounded-bl-[6px] border transition-all duration-300 ${
            isFocused
              ? 'border-sky-400/50 bg-white/80 dark:border-sky-400/30 dark:bg-slate-800/70'
              : 'border-white/60 bg-white/60 dark:border-white/10 dark:bg-slate-800/50'
          } backdrop-blur-xl`}>
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              placeholder={placeholder}
              rows={1}
              className="min-h-[44px] w-full resize-none rounded-t-[22px] bg-transparent px-4 py-3 text-sm leading-7 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
            />

            <AnimatePresence>
              {files.length > 0 && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="flex flex-wrap gap-2 px-4 pb-2"
                >
                  {files.map((file) => (
                    <span
                      key={file.id}
                      className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs ${
                        file.status === 'ready' ? 'border-sky-300/50 bg-sky-50/50 text-sky-600 dark:border-sky-600/30 dark:bg-sky-900/20 dark:text-sky-400' :
                        file.status === 'indexing' ? 'border-amber-300/50 bg-amber-50/50 text-amber-600 dark:border-amber-600/30 dark:bg-amber-900/20 dark:text-amber-400' :
                        file.status === 'failed' ? 'border-rose-300/50 bg-rose-50/50 text-rose-500 dark:border-rose-600/30 dark:bg-rose-900/20' :
                        'border-white/40 bg-white/50 text-zinc-500 dark:border-white/10 dark:bg-slate-700/50'
                      }`}
                    >
                      <FileIcon size={12} />
                      <span className="max-w-[120px] truncate">{file.name}</span>
                      <span className="text-zinc-400">({formatFileSize(file.size)})</span>
                      {file.status === 'indexing' && <Loader2 size={12} className="animate-spin" />}
                      {file.status === 'failed' && <span className="text-[10px]">索引失败</span>}
                      <button
                        onClick={() => removeFile(file.id)}
                        className="ml-0.5 rounded p-0.5 text-zinc-400 transition-colors hover:bg-rose-500/10 hover:text-rose-400"
                      >
                        <X size={12} />
                      </button>
                    </span>
                  ))}
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence>
              {uploadError && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="px-4 pb-2"
                >
                  <p className="text-xs text-rose-400">{uploadError}</p>
                </motion.div>
              )}
            </AnimatePresence>

            <div className="flex items-center justify-between gap-2 border-t border-white/30 px-3 py-2.5 dark:border-white/5 sm:gap-3">
              <div className="flex min-w-0 items-center gap-2 sm:gap-3">
                {isAIOps ? (
                  <div className={`hidden rounded-lg p-0.5 sm:inline-flex ${engineView.sidebar.flowActive}`}>
                    {engineView.flow.map((item) => (
                      <span key={item} className="rounded-md px-2 py-1.5 text-[10px] font-semibold">
                        {item}
                      </span>
                    ))}
                  </div>
                ) : (
                  <div className="inline-flex rounded-lg bg-white/50 p-0.5 dark:bg-slate-700/50">
                    {modeOptions.map((option) => (
                      <button
                        key={option.id}
                        onClick={() => onModeChange(option.id)}
                        className={`flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-all duration-200 sm:px-2.5 ${
                          option.id === mode
                            ? 'bg-white text-sky-600 shadow-sm dark:bg-slate-600 dark:text-sky-400'
                            : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
                        }`}
                      >
                        <option.icon size={13} />
                        <span className="hidden sm:inline">{option.label}</span>
                      </button>
                    ))}
                  </div>
                )}

                <span className={`hidden max-w-[220px] truncate rounded-md px-2 py-1 text-[11px] font-medium sm:inline ${
                  isAIOps
                    ? `${engineView.sidebar.flowActive}`
                    : 'text-zinc-400 dark:text-zinc-600'
                }`}>
                  {contextLabel}
                </span>
              </div>

              <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
                <label htmlFor={inputId}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-white/40 bg-white/50 px-2 py-1.5 text-xs font-medium text-zinc-600 cursor-pointer transition-all hover:-translate-y-0.5 hover:bg-white hover:text-sky-600 hover:shadow-md dark:border-white/10 dark:bg-slate-700/50 dark:text-zinc-400 dark:hover:bg-slate-600 dark:hover:text-sky-400 sm:px-2.5"
                  title="上传文档到知识库">
                  {isUploading ? <Loader2 size={14} className="animate-spin" /> : <Paperclip size={14} />}
                  <span className="hidden sm:inline">上传文档</span>
                </label>

                <span className={`hidden text-[10px] lg:inline ${isAIOps && hasDraft && !aiOpsReady ? 'text-amber-500 dark:text-amber-400' : 'text-zinc-400 dark:text-zinc-600'}`}>
                  {sendHint}
                </span>
                <button
                  onClick={isLoading ? onStop : handleSubmit}
                  className={`inline-flex items-center justify-center gap-2 rounded-xl px-2.5 py-2 text-sm font-semibold transition-all duration-200 sm:px-4 ${
                    isLoading
                      ? 'bg-rose-500/15 text-rose-400 hover:bg-rose-500/25'
                      : canSend
                        ? 'bg-sky-500 text-white shadow-md shadow-sky-400/25 hover:bg-sky-600 hover:shadow-sky-400/40 active:bg-sky-700'
                        : 'cursor-not-allowed bg-white/50 text-zinc-400 dark:bg-slate-700/50 dark:text-zinc-600'
                  }`}
                >
                  {isLoading ? (
                    <>
                      <Square size={14} />
                      <span className="hidden sm:inline">停止</span>
                    </>
                  ) : (
                    <>
                      <Send size={14} />
                      <span className="hidden sm:inline">{submitLabel}</span>
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
