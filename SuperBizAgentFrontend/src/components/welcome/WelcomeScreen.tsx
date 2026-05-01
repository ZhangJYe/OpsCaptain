import { useState, useRef, useEffect } from 'react'
import { motion } from 'framer-motion'
import {
  AlertTriangle, Activity, Shield, Send, Sparkles,
  Search, FileText, Terminal, ArrowUpRight,
} from 'lucide-react'
import { AIOpsPanel } from './AIOpsPanel'

interface Props {
  onSend: (query: string) => void
}

const features = [
  {
    icon: AlertTriangle,
    title: '故障诊断',
    description: '多 Agent 协作分析，并行拉取 metrics、logs、knowledge 三路证据',
    gradient: 'from-amber-500/20 to-orange-500/10',
    iconColor: 'text-amber-400',
  },
  {
    icon: Search,
    title: '根因定位',
    description: '自动关联告警、日志和变更事件，追溯故障传播路径',
    gradient: 'from-blue-500/20 to-cyan-500/10',
    iconColor: 'text-blue-400',
  },
  {
    icon: Shield,
    title: '处置建议',
    description: '基于 SOP 和历史案例，输出可执行的回滚、限流与验证步骤',
    gradient: 'from-emerald-500/20 to-teal-500/10',
    iconColor: 'text-emerald-400',
  },
]

const quickStarts = [
  { icon: Terminal, label: '排查延迟升高', query: '请分析 paymentservice p95 延迟升高的原因，检查错误率、队列堆积和最近变更。' },
  { icon: FileText, label: '分析错误日志', query: '请帮我分析最近的错误日志，找出高频异常模式和影响范围。' },
  { icon: Activity, label: '检查服务健康', query: '请检查当前各服务的健康状态，识别可能的瓶颈和风险点。' },
]

export function WelcomeScreen({ onSend }: Props) {
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
      <div className="mx-auto flex max-w-3xl flex-col items-center px-6 py-12 lg:py-20">

        {/* Hero */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="text-center"
        >
          <div className="inline-flex items-center gap-2 rounded-full border border-accent/20 bg-accent/10 px-4 py-1.5">
            <Sparkles size={14} className="text-accent" />
            <span className="text-xs font-medium text-accent">AI 驱动的智能运维工作台</span>
          </div>
          <h1 className="mt-6 text-4xl font-bold tracking-tight text-zinc-900 dark:text-white sm:text-5xl lg:text-6xl">
            诊断、定位、处置
            <span className="block mt-2 bg-gradient-to-r from-accent to-cyan-400 bg-clip-text text-transparent">
              一站式运维决策
            </span>
          </h1>
          <p className="mt-4 text-base leading-7 text-zinc-500 dark:text-zinc-400 max-w-xl mx-auto">
            描述告警现象或系统异常，OpsCaption 将自动分析影响面、检索证据、对齐历史案例，给出可执行的处置建议。
          </p>
        </motion.div>

        {/* Feature cards */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.1 }}
          className="mt-12 grid w-full gap-4 sm:grid-cols-3"
        >
          {features.map((f) => (
            <div
              key={f.title}
              className={`group relative overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/80 p-5 backdrop-blur transition-all duration-300 hover:-translate-y-1 hover:border-zinc-300 hover:shadow-lg dark:border-zinc-800/60 dark:bg-zinc-900/60 dark:hover:border-zinc-700`}
            >
              <div className={`absolute inset-0 bg-gradient-to-br ${f.gradient} opacity-0 transition-opacity duration-300 group-hover:opacity-100`} />
              <div className="relative">
                <div className={`flex h-10 w-10 items-center justify-center rounded-xl border border-zinc-200/80 bg-white dark:border-zinc-800/60 dark:bg-zinc-900 ${f.iconColor}`}>
                  <f.icon size={20} />
                </div>
                <h3 className="mt-4 text-sm font-semibold text-zinc-900 dark:text-white">{f.title}</h3>
                <p className="mt-1.5 text-xs leading-5 text-zinc-500 dark:text-zinc-400">{f.description}</p>
              </div>
            </div>
          ))}
        </motion.div>

        {/* Quick starts */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.2 }}
          className="mt-8 w-full"
        >
          <p className="mb-3 text-xs font-medium uppercase tracking-wider text-zinc-400 dark:text-zinc-600">
            快速开始
          </p>
          <div className="grid gap-2 sm:grid-cols-3">
            {quickStarts.map((qs) => (
              <button
                key={qs.label}
                onClick={() => onSend(qs.query)}
                className="group flex items-center gap-3 rounded-xl border border-zinc-200/80 bg-white/60 px-4 py-3 text-left transition-all duration-200 hover:border-accent/30 hover:bg-accent/5 dark:border-zinc-800/60 dark:bg-zinc-900/40 dark:hover:border-accent/30"
              >
                <qs.icon size={16} className="shrink-0 text-zinc-400 transition-colors group-hover:text-accent" />
                <span className="flex-1 text-sm text-zinc-600 transition-colors group-hover:text-zinc-900 dark:text-zinc-400 dark:group-hover:text-white">{qs.label}</span>
                <ArrowUpRight size={14} className="shrink-0 text-zinc-300 transition-all group-hover:text-accent group-hover:translate-x-0.5 group-hover:-translate-y-0.5 dark:text-zinc-700" />
              </button>
            ))}
          </div>
        </motion.div>

        {/* AIOps Panel */}
        <AIOpsPanel onStartDiagnosis={onSend} />

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
            <div className="flex items-center justify-between border-t border-zinc-100 px-4 py-3 dark:border-zinc-800">
              <span className="text-[11px] text-zinc-400 dark:text-zinc-600">
                Enter 发送  ·  Shift + Enter 换行
              </span>
              <button
                onClick={handleSubmit}
                disabled={!input.trim()}
                className={`inline-flex items-center gap-2 rounded-xl px-5 py-2 text-sm font-medium transition-all duration-200 ${
                  input.trim()
                    ? 'bg-accent text-white shadow-sm hover:brightness-110 hover:shadow-md active:scale-[0.97]'
                    : 'cursor-not-allowed bg-zinc-100 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-600'
                }`}
              >
                <Send size={14} />
                发送
              </button>
            </div>
          </div>
        </motion.div>

        {/* Footer hint */}
        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.5 }}
          className="mt-8 text-center text-xs text-zinc-400 dark:text-zinc-600"
        >
          建议提供：异常服务名  ·  关键指标变化  ·  近期变更或日志片段
        </motion.p>

      </div>
    </div>
  )
}
