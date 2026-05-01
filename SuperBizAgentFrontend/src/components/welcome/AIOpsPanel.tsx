import { motion } from 'framer-motion'
import {
  AlertTriangle, Activity, Shield, ArrowRight,
  BarChart3, FileSearch, BookOpen, Zap,
} from 'lucide-react'

interface Props {
  onStartDiagnosis: (query: string) => void
}

const metrics = [
  { label: 'Latency', value: 'p95 +42%', note: '10 min', trend: 'up' },
  { label: 'Errors', value: '2.7%', note: 'checkout', trend: 'up' },
  { label: 'Queue', value: '+18%', note: 'retry', trend: 'up' },
  { label: 'Evidence', value: '3/4', note: 'ready', trend: 'stable' },
]

const evidence = [
  { icon: BarChart3, label: 'Metrics', status: '已拉取', detail: 'p95、retry、queue depth', color: 'text-blue-400', bg: 'bg-blue-500/10', dot: 'bg-blue-400' },
  { icon: FileSearch, label: 'Logs', status: '进行中', detail: 'Redis timeout 集中出现', color: 'text-amber-400', bg: 'bg-amber-500/10', dot: 'bg-amber-400' },
  { icon: BookOpen, label: 'Knowledge', status: '已匹配', detail: '支付超时 SOP + 历史案例', color: 'text-emerald-400', bg: 'bg-emerald-500/10', dot: 'bg-emerald-400' },
]

const steps = [
  '先判断错误率和队列堆积是否同步扩大',
  '确认 Redis / DB timeout 是否在 checkout path 集中出现',
  '输出回滚、限流与验证步骤，标注风险',
]

const diagnosisQuery = `请对 paymentservice 进行一次完整的 AIOps 诊断：

1. 影响判断：分析 p95 延迟升高 +42% 的影响范围，判断错误率(2.7%)和队列堆积(+18%)是否同步扩大
2. 证据对照：并行对比 metrics（延迟/错误/队列）、logs（Redis timeout 分布）、knowledge（支付超时 SOP）
3. 处置建议：根据证据给出回滚、限流和验证步骤，标注每步风险等级

不要先建议重启。优先判断 timeout 和重试是否同时抬升。`

export function AIOpsPanel({ onStartDiagnosis }: Props) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, delay: 0.15 }}
      className="w-full"
    >
      <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/90 shadow-sm backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/70">
        {/* Header — incident summary */}
        <div className="border-b border-zinc-100 bg-zinc-50/80 px-5 py-4 dark:border-zinc-800 dark:bg-zinc-950/50">
          <div className="flex flex-wrap items-center gap-3 text-xs">
            <span className="inline-flex items-center gap-1.5 rounded-full border border-red-500/30 bg-red-500/10 px-2.5 py-1 font-semibold text-red-400">
              <span className="h-2 w-2 rounded-full bg-red-400" />
              SEV-2
            </span>
            <span className="font-medium text-zinc-600 dark:text-zinc-400">prod / paymentservice</span>
            <span className="text-zinc-400 dark:text-zinc-600">林澈值班</span>
            <span className="ml-auto text-zinc-400 dark:text-zinc-600">analysis window 10min</span>
          </div>
          <h2 className="mt-3 text-xl font-bold tracking-tight text-zinc-900 dark:text-white">
            Paymentservice 延迟异常
          </h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500 dark:text-zinc-400">
            p95 在近 10 分钟窗口内抬升。先确认影响面，再对齐 metrics、error logs、最近变更与历史相似案例，最后整理回滚和验证步骤。
          </p>
        </div>

        {/* Metric tiles */}
        <div className="grid grid-cols-2 gap-px border-b border-zinc-100 bg-zinc-100 dark:border-zinc-800 dark:bg-zinc-800 sm:grid-cols-4">
          {metrics.map((m) => (
            <div key={m.label} className="bg-white/90 px-4 py-3.5 dark:bg-zinc-900/70">
              <div className="text-[10px] font-semibold uppercase tracking-wider text-zinc-400 dark:text-zinc-600">
                {m.label}
              </div>
              <div className="mt-1.5 flex items-baseline gap-1.5">
                <span className="text-xl font-bold text-zinc-900 dark:text-white">{m.value}</span>
                {m.trend === 'up' && (
                  <span className="text-[10px] font-medium text-red-400">↑</span>
                )}
              </div>
              <div className="mt-0.5 text-[10px] text-zinc-400 dark:text-zinc-600">{m.note}</div>
            </div>
          ))}
        </div>

        {/* Evidence + workflow */}
        <div className="grid gap-px border-b border-zinc-100 bg-zinc-100 dark:border-zinc-800 dark:bg-zinc-800 lg:grid-cols-[1fr_1.1fr]">
          {/* Evidence */}
          <div className="bg-white/90 p-4 dark:bg-zinc-900/70">
            <div className="flex items-center gap-2 text-xs font-semibold text-zinc-600 dark:text-zinc-400">
              <Activity size={13} className="text-accent" />
              证据链
              <span className="ml-auto text-[10px] font-normal text-zinc-400 dark:text-zinc-600">3 路并行</span>
            </div>
            <div className="mt-3 space-y-2.5">
              {evidence.map((e) => (
                <div key={e.label} className="flex items-start gap-3 rounded-lg p-2.5 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/50">
                  <div className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg ${e.bg}`}>
                    <e.icon size={14} className={e.color} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-zinc-800 dark:text-zinc-200">{e.label}</span>
                      <span className={`h-1.5 w-1.5 rounded-full ${e.dot}`} />
                      <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{e.status}</span>
                    </div>
                    <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-500">{e.detail}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Workflow */}
          <div className="bg-white/90 p-4 dark:bg-zinc-900/70">
            <div className="flex items-center gap-2 text-xs font-semibold text-zinc-600 dark:text-zinc-400">
              <Shield size={13} className="text-accent" />
              建议节奏
              <span className="ml-auto text-[10px] font-normal text-zinc-400 dark:text-zinc-600">safe path</span>
            </div>
            <ol className="mt-3 space-y-2.5">
              {steps.map((step, i) => (
                <li key={i} className="flex items-start gap-2.5 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
                  <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent/10 text-[11px] font-semibold text-accent">
                    {i + 1}
                  </span>
                  {step}
                </li>
              ))}
            </ol>
            <div className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/5 px-3.5 py-2.5">
              <div className="flex items-start gap-2">
                <AlertTriangle size={14} className="mt-0.5 shrink-0 text-amber-400" />
                <p className="text-xs leading-5 text-amber-600 dark:text-amber-400">
                  不要先重启。先看 timeout 和重试是否同时抬升，再判断是否需要回滚。
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Action */}
        <div className="flex items-center justify-between px-5 py-4">
          <p className="text-xs text-zinc-400 dark:text-zinc-600">
            AI Agent 将自动并行拉取三路证据
          </p>
          <button
            onClick={() => onStartDiagnosis(diagnosisQuery)}
            className="group inline-flex items-center gap-2 rounded-xl bg-accent px-5 py-2.5 text-sm font-semibold text-white shadow-sm shadow-accent/25 transition-all duration-200 hover:brightness-110 hover:shadow-md active:scale-[0.97]"
          >
            <Zap size={15} />
            启动 AIOps 诊断
            <ArrowRight size={14} className="transition-transform duration-200 group-hover:translate-x-0.5" />
          </button>
        </div>
      </div>
    </motion.div>
  )
}
