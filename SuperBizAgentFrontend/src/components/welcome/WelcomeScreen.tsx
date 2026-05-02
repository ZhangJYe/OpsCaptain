import { useState, useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Send, Paperclip, X, Loader2, FileIcon, ArrowRight, AlertTriangle, Activity, BookOpen } from 'lucide-react'
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
  'paymentservice 延迟升高，先看错误率和队列堆积',
  '请分析 checkout path 最近的 timeout 日志',
  '帮我检索支付超时相关 SOP 和历史案例',
  '请给出回滚、限流和验证步骤',
]

const workbenchNotes = [
  {
    icon: AlertTriangle,
    label: 'Incident',
    value: '从告警、报错或异常现象开始',
  },
  {
    icon: Activity,
    label: 'Evidence',
    value: '优先对齐 metrics、logs、knowledge',
  },
  {
    icon: BookOpen,
    label: 'Output',
    value: '结论、原因、处置建议要分层',
  },
]

const aiopsDraftQuery = `请按一次真实值班排障的方式分析这个问题：

- 先判断影响范围和风险等级
- 再对齐 metrics、logs、knowledge 三路证据
- 最后给出回滚、限流和验证步骤

异常现象：paymentservice p95 延迟升高，错误率开始抬升，checkout path 出现 timeout。`

export function WelcomeScreen({ onSend }: Props) {
  const [input, setInput] = useState('')
  const [isFocused, setIsFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const { files, isUploading, uploadError, removeFile, clearFiles, inputId, handleChange, accept, multiple } = useFileUpload()

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
      <input type="file" id={inputId} onChange={handleChange} accept={accept} multiple={multiple} className="hidden" />

      <div className="mx-auto flex max-w-4xl flex-col px-6 py-10 lg:py-14">
        <motion.div
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="mb-8"
        >
          <div className="mb-3 text-[11px] font-medium uppercase tracking-[0.22em] text-zinc-400 dark:text-zinc-600">
            OpsCaption / AI Workbench
          </div>
          <h1 className="max-w-3xl text-[2rem] font-semibold tracking-[-0.04em] text-zinc-950 dark:text-zinc-50 sm:text-[2.5rem]">
            先给我现象，我去收证据。
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-zinc-500 dark:text-zinc-400">
            直接贴告警、错误日志、服务名、变更信息，或者上传文档。界面尽量少说话，把注意力留给诊断过程本身。
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, delay: 0.05 }}
          className="mb-6 grid gap-3 sm:grid-cols-3"
        >
          {workbenchNotes.map((note) => (
            <div
              key={note.label}
              className="rounded-2xl border border-zinc-200/80 bg-white/70 px-4 py-3 dark:border-zinc-800/60 dark:bg-zinc-900/50"
            >
              <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em] text-zinc-400 dark:text-zinc-600">
                <note.icon size={13} className="text-accent" />
                {note.label}
              </div>
              <div className="mt-2 text-sm leading-6 text-zinc-700 dark:text-zinc-300">{note.value}</div>
            </div>
          ))}
        </motion.div>

        {/* Input area */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="w-full"
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
              <div className="flex items-center gap-3">
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
          <div className="mb-2 text-[11px] text-zinc-400 dark:text-zinc-600">可以这样开始</div>
          <div className="flex flex-wrap gap-2">
            {quickStarters.map((starter) => (
              <button
                key={starter}
                onClick={() => onSend(starter)}
                className="rounded-full border border-zinc-200/80 bg-white/70 px-3 py-2 text-xs text-zinc-600 transition-colors hover:border-accent/30 hover:text-accent dark:border-zinc-800/60 dark:bg-zinc-900/50 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
              >
                {starter}
              </button>
            ))}
          </div>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, delay: 0.22 }}
          className="mt-6 overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 dark:border-zinc-800/60 dark:bg-zinc-900/50"
        >
          <div className="flex items-center justify-between border-b border-zinc-100 px-4 py-3 dark:border-zinc-800">
            <div className="text-xs font-medium text-zinc-500 dark:text-zinc-400">AIOps Draft</div>
            <button
              onClick={() => onSend(aiopsDraftQuery)}
              className="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium text-zinc-600 transition-colors hover:bg-zinc-100 hover:text-accent dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-accent"
            >
              直接开始
              <ArrowRight size={13} />
            </button>
          </div>
          <pre className="overflow-x-auto whitespace-pre-wrap px-4 py-4 text-[12px] leading-6 text-zinc-600 dark:text-zinc-400">
{aiopsDraftQuery}
          </pre>
        </motion.div>

        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.28 }}
          className="mt-5 text-xs text-zinc-400 dark:text-zinc-600"
        >
          支持上传 .md .txt .pdf .csv .json .yaml 到知识库
        </motion.p>
      </div>
    </div>
  )
}
