import { useState, useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Send, Paperclip, X, Loader2, FileIcon } from 'lucide-react'
import { AIOpsPanel } from './AIOpsPanel'
import { AgentGreeting } from '../agent/AgentGreeting'
import { useFileUpload } from '../../hooks/useFileUpload'
import { formatFileSize } from '../../lib/utils'

interface Props {
  onSend: (query: string) => void
}

function buildQueryWithFiles(query: string, fileNames: string[]): string {
  if (fileNames.length === 0) return query
  const refs = fileNames.map((n) => `[已上传: ${n}]`).join('\n')
  return `${refs}\n\n${query}`
}

export function WelcomeScreen({ onSend }: Props) {
  const [input, setInput] = useState('')
  const [isFocused, setIsFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const { files, isUploading, uploadError, openFilePicker, removeFile, clearFiles, fileInputProps } = useFileUpload()

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const handleSubmit = () => {
    if (!input.trim() && files.length === 0) return
    const names = files.map((f) => f.name)
    const query = buildQueryWithFiles(input.trim(), names)
    onSend(query || '请分析上传的文件')
    setTimeout(() => setInput(''), 0)
    clearFiles()
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  const canSend = input.trim().length > 0 || files.length > 0

  return (
    <div className="h-full overflow-y-auto scrollbar-thin">
      {/* Hidden file input */}
      <input {...fileInputProps} />

      <div className="mx-auto flex max-w-3xl flex-col items-center px-6 py-12 lg:py-20">

        {/* Agent greeting */}
        <AgentGreeting onSuggestion={onSend} />

        {/* AIOps Panel */}
        <div className="mt-10 w-full">
          <AIOpsPanel onStartDiagnosis={onSend} />
        </div>

        {/* Input area */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="mt-10 w-full"
        >
          <div
            className={`rounded-2xl border transition-all duration-300 ${
              isFocused
                ? 'border-accent/40 shadow-[0_0_0_4px_rgba(59,130,246,0.1)] dark:shadow-[0_0_0_4px_rgba(59,130,246,0.08)]'
                : 'border-zinc-200/80 shadow-sm dark:border-zinc-800/60'
            } bg-white/90 backdrop-blur dark:bg-zinc-900/70`}
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
              className="min-h-[52px] w-full resize-none bg-transparent px-5 py-4 text-sm leading-6 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
            />

            <AnimatePresence>
              {files.length > 0 && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="flex flex-wrap gap-2 px-5 pb-2"
                >
                  {files.map((file) => (
                    <span key={file.id} className="inline-flex items-center gap-1.5 rounded-lg border border-accent/30 bg-accent/5 px-2.5 py-1 text-xs text-accent">
                      <FileIcon size={12} />
                      <span className="max-w-[140px] truncate">{file.name}</span>
                      <span className="text-zinc-400">({formatFileSize(file.size)})</span>
                      <button onClick={() => removeFile(file.id)} className="ml-0.5 rounded p-0.5 text-zinc-400 transition-colors hover:bg-red-500/10 hover:text-red-400">
                        <X size={12} />
                      </button>
                    </span>
                  ))}
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence>
              {uploadError && (
                <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }} className="px-5 pb-2">
                  <p className="text-xs text-red-400">{uploadError}</p>
                </motion.div>
              )}
            </AnimatePresence>

            <div className="flex items-center justify-between border-t border-zinc-100 px-4 py-3 dark:border-zinc-800">
              <div className="flex items-center gap-2">
                <button onClick={openFilePicker} disabled={isUploading} className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-zinc-400" title="上传知识库文件">
                  {isUploading ? <Loader2 size={16} className="animate-spin" /> : <Paperclip size={16} />}
                </button>
                <span className="text-[11px] text-zinc-400 dark:text-zinc-600">Enter 发送 · Shift+Enter 换行</span>
              </div>
              <button onClick={handleSubmit} disabled={!canSend}
                className={`inline-flex items-center gap-2 rounded-xl px-5 py-2 text-sm font-medium transition-all duration-200 ${
                  canSend ? 'bg-accent text-white shadow-sm hover:brightness-110 hover:shadow-md active:scale-[0.97]' : 'cursor-not-allowed bg-zinc-100 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-600'
                }`}>
                <Send size={14} />发送
              </button>
            </div>
          </div>
        </motion.div>

        <motion.p initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.5, delay: 0.5 }}
          className="mt-8 text-center text-xs text-zinc-400 dark:text-zinc-600">
          可上传 .md .txt .pdf .csv .json .yaml 到知识库 · 支持拖拽
        </motion.p>
      </div>
    </div>
  )
}
