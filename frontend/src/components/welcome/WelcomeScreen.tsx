import { useState, useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Send, Paperclip, X, Loader2, FileIcon, Search, Database, MessageSquareText, ShieldCheck } from 'lucide-react'
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

const quickStarters = [
  '分析 paymentservice p95 延迟升高，先看错误率和队列堆积',
  '检索 checkout path 最近 timeout 相关日志和历史回答',
  '根据支付超时 SOP 总结排查顺序和验证项',
  '把这段告警整理成影响范围、可能原因和下一步动作',
]

const workbenchNotes = [
  {
    icon: Search,
    label: 'Context',
    value: '先理解问题和约束',
  },
  {
    icon: Database,
    label: 'Evidence',
    value: '补齐历史、知识库和文件',
  },
  {
    icon: ShieldCheck,
    label: 'Answer',
    value: '给出结论、证据和动作',
  },
]

const contextSteps = [
  '识别服务、时间窗、错误类型和影响面',
  '关联已选能力、会话记忆和上传文档',
  '输出可复核的结论与后续追问建议',
]

export function WelcomeScreen({ onSend }: Props) {
  const [input, setInput] = useState('')
  const [isFocused, setIsFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const { files, readyFiles, isUploading, uploadError, removeFile, clearFiles, inputId, handleChange, accept, multiple } = useFileUpload()

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const handleSubmit = () => {
    if (!input.trim() && readyFiles.length === 0) return
    const names = readyFiles.map((f) => f.name)
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

  const canSend = input.trim().length > 0 || readyFiles.length > 0

  return (
    <div className="h-full overflow-y-auto bg-[radial-gradient(circle_at_50%_0%,rgba(59,130,246,0.08),transparent_32%),linear-gradient(180deg,#ffffff_0%,#fafafa_52%,#f6f8fb_100%)] scrollbar-thin dark:bg-[radial-gradient(circle_at_50%_0%,rgba(59,130,246,0.13),transparent_34%),linear-gradient(180deg,#09090b_0%,#0b0d10_100%)]">
      <input type="file" id={inputId} onChange={handleChange} accept={accept} multiple={multiple} className="hidden" />

      <div className="mx-auto flex min-h-full max-w-5xl flex-col px-4 py-8 sm:px-6 lg:py-10">
        <motion.div
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-end"
        >
          <div>
            <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1.5 text-[11px] font-medium text-zinc-500 shadow-sm dark:border-zinc-800/70 dark:bg-zinc-900/70 dark:text-zinc-400">
              <MessageSquareText size={13} className="text-accent" />
              ReAct 问答工作台
            </div>
            <h1 className="max-w-3xl text-[2rem] font-semibold tracking-normal text-zinc-950 dark:text-zinc-50 sm:text-[2.45rem]">
              描述问题，OpsCaption 会先收集上下文再回答。
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-zinc-500 dark:text-zinc-400">
              适合日常问答、知识检索、日志片段分析和文档归纳。事故排障入口保留在左侧模式切换里，问答首页只呈现问答能力。
            </p>
          </div>
          <div className="rounded-2xl border border-zinc-200/80 bg-white/76 p-4 shadow-sm shadow-zinc-900/[0.03] backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/54">
            <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-zinc-400 dark:text-zinc-600">Context Loop</div>
            <div className="mt-3 space-y-2.5">
              {contextSteps.map((step, index) => (
                <div key={step} className="flex items-start gap-3">
                  <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent/10 text-[11px] font-semibold text-accent">
                    {index + 1}
                  </span>
                  <span className="text-sm leading-6 text-zinc-600 dark:text-zinc-300">{step}</span>
                </div>
              ))}
            </div>
          </div>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, delay: 0.05 }}
          className="mt-6 grid gap-3 sm:grid-cols-3"
        >
          {workbenchNotes.map((note) => (
            <div
              key={note.label}
              className="rounded-xl border border-zinc-200/80 bg-white/74 px-4 py-3 shadow-sm shadow-zinc-900/[0.02] backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/50"
            >
              <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-400 dark:text-zinc-600">
                <note.icon size={13} className="text-accent" />
                {note.label}
              </div>
              <div className="mt-2 text-sm leading-6 text-zinc-700 dark:text-zinc-300">{note.value}</div>
            </div>
          ))}
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="mt-5 w-full"
        >
          <div
            className={`rounded-2xl border transition-all duration-300 ${
              isFocused
                ? 'border-accent/45 shadow-[0_18px_52px_rgba(59,130,246,0.13)] dark:shadow-[0_18px_52px_rgba(59,130,246,0.08)]'
                : 'border-zinc-200/80 shadow-lg shadow-zinc-900/[0.04] dark:border-zinc-800/60 dark:shadow-black/20'
            } bg-white/94 backdrop-blur dark:bg-zinc-900/74`}
          >
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              placeholder="输入问题、日志片段、服务名或知识库检索需求..."
              rows={1}
              className="min-h-[74px] w-full resize-none bg-transparent px-5 py-4 text-sm leading-7 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
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
                    <span key={file.id} className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs ${
                      file.status === 'ready' ? 'border-accent/30 bg-accent/5 text-accent' :
                      file.status === 'indexing' ? 'border-yellow-300/50 bg-yellow-50/50 text-yellow-600 dark:border-yellow-600/30 dark:bg-yellow-900/20 dark:text-yellow-400' :
                      file.status === 'failed' ? 'border-red-300/50 bg-red-50/50 text-red-500 dark:border-red-600/30 dark:bg-red-900/20' :
                      'border-zinc-200 bg-zinc-50 text-zinc-500 dark:border-zinc-700 dark:bg-zinc-800'
                    }`}>
                      <FileIcon size={12} />
                      <span className="max-w-[140px] truncate">{file.name}</span>
                      <span className="text-zinc-400">({formatFileSize(file.size)})</span>
                      {file.status === 'indexing' && <Loader2 size={12} className="animate-spin" />}
                      {file.status === 'failed' && <span className="text-[10px]">索引失败</span>}
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

            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-zinc-100 px-4 py-3 dark:border-zinc-800">
              <div className="flex flex-wrap items-center gap-3">
                <label htmlFor={inputId}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-zinc-200/80 bg-white px-2.5 py-1.5 text-xs font-medium text-zinc-600 cursor-pointer transition-all hover:border-accent/30 hover:text-accent dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
                  title="上传文档到知识库">
                  {isUploading ? <Loader2 size={14} className="animate-spin" /> : <Paperclip size={14} />}
                  上传文档
                </label>
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

        <motion.div
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, delay: 0.15 }}
          className="mt-5"
        >
          <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-400 dark:text-zinc-600">Quick Prompts</div>
          <div className="flex flex-wrap gap-2">
            {quickStarters.map((starter) => (
              <button
                key={starter}
                onClick={() => onSend(starter)}
                className="rounded-full border border-zinc-200/80 bg-white/74 px-3 py-2 text-xs text-zinc-600 shadow-sm shadow-zinc-900/[0.02] transition-colors hover:border-accent/30 hover:bg-accent/5 hover:text-accent dark:border-zinc-800/60 dark:bg-zinc-900/50 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
              >
                {starter}
              </button>
            ))}
          </div>
        </motion.div>

        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.28 }}
          className="mt-auto pt-6 text-xs text-zinc-400 dark:text-zinc-600"
        >
          支持上传 .md .txt .pdf .csv .json .yaml 到知识库
        </motion.p>
      </div>
    </div>
  )
}
