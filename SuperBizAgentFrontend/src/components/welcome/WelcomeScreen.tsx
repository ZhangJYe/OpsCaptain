import { useState, useRef, useEffect } from 'react'
import { motion } from 'framer-motion'
import { Activity, AlertTriangle, ArrowRight, Clock3, Database, Send, Shield, TerminalSquare } from 'lucide-react'

interface Props {
  onSend: (query: string) => void
}

const metricTiles = [
  { label: 'Latency', value: 'p95 +42%', note: '10 min window' },
  { label: 'Errors', value: '2.7%', note: 'checkout path' },
  { label: 'Queue', value: '+18%', note: 'retry backlog' },
  { label: 'Evidence', value: '3 / 4', note: 'ready to reason' },
]

const evidenceRows = [
  { title: 'Metrics', detail: 'p95、retry、queue depth 已拉取', tone: 'bg-emerald-400' },
  { title: 'Logs', detail: 'Redis timeout 在 checkout path 集中出现', tone: 'bg-accent' },
  { title: 'Knowledge', detail: '支付超时 SOP 和历史相似案例已匹配', tone: 'bg-amber-400' },
]

const quickActions = [
  {
    title: '故障诊断',
    description: '按影响面、依赖链路和历史案例给出处理建议',
    icon: AlertTriangle,
    action: '请模拟一次 paymentservice 延迟升高的线上故障诊断，输出影响判断、证据检查、可能原因和处理建议。',
  },
  {
    title: '证据对照',
    description: '并行对比 metrics、logs、knowledge 三路证据',
    icon: Activity,
    action: '请按 metrics、logs、knowledge 三路证据，分析 paymentservice p95 升高可能原因。',
  },
  {
    title: '处置建议',
    description: '给出回滚、限流和验证步骤，并标注风险',
    icon: Shield,
    action: '请给出 paymentservice 延迟升高时的回滚、限流和验证步骤。',
  },
]

const operatorNotes = [
  '先判断影响范围，再下结论',
  '历史案例只作辅助证据，不直接替代实时信号',
  '需要回滚时，优先给出验证步骤和风险说明',
]

export function WelcomeScreen({ onSend }: Props) {
  const [input, setInput] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const handleSubmit = () => {
    if (!input.trim()) return
    onSend(input.trim())
    setTimeout(() => setInput(''), 0)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }
  return (
    <div className="h-full overflow-y-auto scrollbar-thin">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 lg:px-6 lg:py-8">
        <motion.section
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45 }}
          className="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_340px]"
        >
          <div className="rounded-[28px] border border-zinc-200/80 bg-white/92 p-6 shadow-[0_28px_120px_rgba(15,23,42,0.10)] backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/80 dark:shadow-[0_28px_120px_rgba(0,0,0,0.34)] lg:p-8">
            <div className="flex flex-wrap items-center gap-3 text-xs text-zinc-500 dark:text-zinc-400">
              <span className="inline-flex items-center gap-2 rounded-full border border-red-500/30 bg-red-500/10 px-3 py-1 font-medium text-red-300">
                <span className="h-2 w-2 rounded-full bg-red-400" />
                SEV-2
              </span>
              <span>prod / paymentservice</span>
              <span>林澈值班</span>
              <span className="inline-flex items-center gap-1 text-zinc-500">
                <Clock3 size={12} />
                analysis window 10m
              </span>
            </div>

            <div className="mt-6 max-w-3xl">
              <h1 className="text-4xl font-semibold tracking-[-0.04em] leading-[0.96] text-zinc-950 dark:text-white sm:text-5xl lg:text-[4.75rem] xl:text-[5.35rem]">
                Paymentservice 延迟异常
              </h1>
              <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400 lg:text-base">
                p95 在近 10 分钟窗口内抬升。先确认影响面，再对齐 metrics、error logs、最近变更与历史相似案例，最后整理回滚和验证步骤。
              </p>
            </div>

            <div className="mt-8 grid grid-cols-2 gap-3 lg:grid-cols-4">
              {metricTiles.map((tile) => (
                <div
                  key={tile.label}
                  className="rounded-2xl border border-zinc-200/80 bg-zinc-50/92 px-4 py-4 dark:border-zinc-800/80 dark:bg-zinc-900/62"
                >
                  <div className="text-[11px] uppercase tracking-[0.18em] text-zinc-500 dark:text-zinc-500">{tile.label}</div>
                  <div className="mt-2 text-2xl font-semibold text-zinc-900 dark:text-zinc-100">{tile.value}</div>
                  <div className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">{tile.note}</div>
                </div>
              ))}
            </div>

            <div className="mt-8 grid gap-4 lg:grid-cols-[minmax(0,1.06fr)_minmax(0,0.94fr)]">
              <div className="rounded-2xl border border-zinc-200/80 bg-zinc-50/92 p-5 dark:border-zinc-800/80 dark:bg-zinc-900/54">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                    <Database size={16} className="text-accent" />
                    证据链
                  </div>
                  <span className="text-xs text-zinc-500 dark:text-zinc-500">3 ready</span>
                </div>
                <div className="mt-5 space-y-4">
                  {evidenceRows.map((row) => (
                    <div key={row.title} className="flex items-start gap-3">
                      <span className={`mt-2 h-2.5 w-2.5 rounded-full ${row.tone}`} />
                      <div>
                        <div className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{row.title}</div>
                        <div className="mt-1 text-sm leading-6 text-zinc-600 dark:text-zinc-400">{row.detail}</div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className="rounded-2xl border border-zinc-200/80 bg-zinc-50/92 p-5 dark:border-zinc-800/80 dark:bg-zinc-900/54">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                    <TerminalSquare size={16} className="text-accent" />
                    建议节奏
                  </div>
                  <span className="text-xs text-zinc-500 dark:text-zinc-500">safe path</span>
                </div>
                <ol className="mt-5 space-y-4 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
                  <li>1. 先判断错误率和队列堆积是否同步扩大。</li>
                  <li>2. 再确认 Redis / DB timeout 是否在 checkout path 集中出现。</li>
                  <li>3. 最后输出回滚、限流与验证步骤，并标注风险。</li>
                </ol>
                <div className="mt-5 rounded-2xl border border-accent/20 bg-accent/8 px-4 py-3 text-sm leading-6 text-zinc-700 dark:text-zinc-300">
                  不要先重启。先看 timeout 和重试是否同时抬升，再判断是否需要回滚。
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-4">
            <div className="rounded-[24px] border border-zinc-200/80 bg-white/92 p-5 backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/80">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase tracking-[0.22em] text-zinc-500 dark:text-zinc-600">Current Shift</div>
                  <div className="mt-2 text-xl font-semibold text-zinc-900 dark:text-zinc-100">林澈</div>
                </div>
                <span className="inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-xs font-medium text-emerald-300">
                  <span className="h-2 w-2 rounded-full bg-emerald-400" />
                  Live
                </span>
              </div>
              <p className="mt-4 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
                整理证据、判断影响面，再把结论收束成可以执行的处置建议。
              </p>
            </div>

            <div className="rounded-[24px] border border-zinc-200/80 bg-white/92 p-5 backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/80">
              <div className="text-xs uppercase tracking-[0.22em] text-zinc-500 dark:text-zinc-600">Current Guardrail</div>
              <ul className="mt-4 space-y-3 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
                {operatorNotes.map((note) => (
                  <li key={note} className="flex items-start gap-3">
                    <span className="mt-2 h-1.5 w-1.5 rounded-full bg-accent" />
                    <span>{note}</span>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </motion.section>

        <motion.section
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45, delay: 0.12 }}
          className="grid gap-4 lg:grid-cols-[minmax(0,1.3fr)_320px]"
        >
          <div className="rounded-[24px] border border-zinc-200/80 bg-white/92 p-4 backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/70 lg:p-5">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <div className="text-xs uppercase tracking-[0.22em] text-zinc-500 dark:text-zinc-600">Quick Entry</div>
                <div className="mt-2 text-lg font-semibold text-zinc-900 dark:text-zinc-100">从一个明确动作开始</div>
              </div>
              <div className="hidden text-xs text-zinc-500 dark:text-zinc-500 md:block">支持 metrics / logs / knowledge 联动</div>
            </div>
            <div className="grid gap-3 lg:grid-cols-3">
              {quickActions.map((item) => (
                <button
                  key={item.title}
                  onClick={() => onSend(item.action)}
                  className="group rounded-2xl border border-zinc-200/80 bg-zinc-50/92 p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-zinc-300 hover:bg-white dark:border-zinc-800/80 dark:bg-zinc-900/50 dark:hover:border-zinc-700 dark:hover:bg-zinc-900/70"
                >
                  <div className="flex items-center gap-3">
                    <div className="flex h-10 w-10 items-center justify-center rounded-2xl border border-accent/20 bg-accent/10 text-accent">
                      <item.icon size={18} />
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{item.title}</div>
                      <div className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-500">{item.description}</div>
                    </div>
                  </div>
                  <div className="mt-5 inline-flex items-center gap-2 text-xs font-medium text-accent">
                    立即开始
                    <ArrowRight size={14} className="transition-transform duration-200 group-hover:translate-x-1" />
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="rounded-[24px] border border-zinc-200/80 bg-white/92 p-5 backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/70">
            <div className="text-xs uppercase tracking-[0.22em] text-zinc-500 dark:text-zinc-600">Input Hint</div>
            <div className="mt-3 text-lg font-semibold text-zinc-900 dark:text-zinc-100">给出三个关键信号</div>
            <ul className="mt-4 space-y-3 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
              <li className="flex items-start gap-3">
                <span className="mt-2 h-1.5 w-1.5 rounded-full bg-accent" />
                <span>异常服务或告警名称</span>
              </li>
              <li className="flex items-start gap-3">
                <span className="mt-2 h-1.5 w-1.5 rounded-full bg-accent" />
                <span>影响指标，例如延迟、错误率、重试、队列</span>
              </li>
              <li className="flex items-start gap-3">
                <span className="mt-2 h-1.5 w-1.5 rounded-full bg-accent" />
                <span>最近发布、依赖变更或错误日志片段</span>
              </li>
            </ul>
          </div>
        </motion.section>

        <motion.section
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45, delay: 0.18 }}
        >
          <div className="rounded-[24px] border border-zinc-200/80 bg-white/92 p-4 backdrop-blur-xl dark:border-zinc-800/80 dark:bg-zinc-950/70">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="描述告警、日志或系统现象，按 Enter 发送..."
              rows={1}
              className="min-h-[44px] w-full resize-none bg-transparent px-2 py-2 text-sm leading-7 text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100 dark:placeholder:text-zinc-500"
            />
            <div className="flex items-center justify-between border-t border-zinc-200/80 pt-3 dark:border-zinc-800/80">
              <span className="text-[11px] text-zinc-500 dark:text-zinc-500">Enter 发送，Shift + Enter 换行</span>
              <button
                onClick={handleSubmit}
                className={`inline-flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-medium transition-all ${
                  input.trim()
                    ? 'bg-accent text-white hover:brightness-110'
                    : 'bg-zinc-100 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-600'
                }`}
              >
                <Send size={14} />
                发送
              </button>
            </div>
          </div>
        </motion.section>
      </div>
    </div>
  )
}
