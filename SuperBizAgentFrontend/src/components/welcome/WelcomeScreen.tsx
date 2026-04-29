import { motion } from 'framer-motion'
import { AlertTriangle, TrendingUp, Activity, Shield } from 'lucide-react'

interface Props {
  onSend: (query: string) => void
}

export function WelcomeScreen({ onSend }: Props) {
  return (
    <div className="h-full overflow-y-auto scrollbar-thin">
      <div className="max-w-3xl mx-auto px-4 py-10">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="text-center mb-8"
        >
          <div className="w-16 h-16 rounded-2xl bg-accent/20 border border-accent/30 flex items-center justify-center mx-auto mb-4">
            <AlertTriangle size={32} className="text-accent" />
          </div>
          <h1 className="text-2xl font-bold mb-2">OpsCaption 运维诊断</h1>
          <p className="text-zinc-500 text-sm">
            告警分析 · 日志排查 · 知识检索 · 多 Agent 协同
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.15 }}
          className="glass rounded-2xl p-6 mb-6"
        >
          <h2 className="text-sm font-semibold mb-4 flex items-center gap-2">
            <Activity size={16} className="text-accent" />
            快速诊断入口
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <button
              onClick={() => onSend('请模拟一次 paymentservice 延迟升高的线上故障诊断，输出影响判断、证据检查、可能原因和处理建议。')}
              className="glass-hover rounded-xl p-4 text-left transition-all duration-200 hover:-translate-y-0.5"
            >
              <div className="flex items-center gap-2 mb-1">
                <div className="w-6 h-6 rounded-lg bg-red-500/20 flex items-center justify-center">
                  <AlertTriangle size={14} className="text-red-400" />
                </div>
                <span className="text-sm font-medium">故障诊断</span>
              </div>
              <p className="text-xs text-zinc-500">Paymentservice 延迟升高，多路证据交叉验证</p>
            </button>

            <button
              onClick={() => onSend('请按 metrics、logs、knowledge 三路证据，分析 paymentservice p95 升高可能原因。')}
              className="glass-hover rounded-xl p-4 text-left transition-all duration-200 hover:-translate-y-0.5"
            >
              <div className="flex items-center gap-2 mb-1">
                <div className="w-6 h-6 rounded-lg bg-blue-500/20 flex items-center justify-center">
                  <TrendingUp size={14} className="text-blue-400" />
                </div>
                <span className="text-sm font-medium">证据对照</span>
              </div>
              <p className="text-xs text-zinc-500">Metrics + Logs + Knowledge 三路并行</p>
            </button>

            <button
              onClick={() => onSend('请给出 paymentservice 延迟升高时的回滚、限流和验证步骤。')}
              className="glass-hover rounded-xl p-4 text-left transition-all duration-200 hover:-translate-y-0.5"
            >
              <div className="flex items-center gap-2 mb-1">
                <div className="w-6 h-6 rounded-lg bg-amber-500/20 flex items-center justify-center">
                  <Shield size={14} className="text-amber-400" />
                </div>
                <span className="text-sm font-medium">处置建议</span>
              </div>
              <p className="text-xs text-zinc-500">回滚策略、限流方案、验证步骤</p>
            </button>

            <button
              onClick={() => onSend('帮我分析最近一次部署的健康状况，包括告警、错误率和响应时间。')}
              className="glass-hover rounded-xl p-4 text-left transition-all duration-200 hover:-translate-y-0.5"
            >
              <div className="flex items-center gap-2 mb-1">
                <div className="w-6 h-6 rounded-lg bg-emerald-500/20 flex items-center justify-center">
                  <Activity size={14} className="text-emerald-400" />
                </div>
                <span className="text-sm font-medium">发布护航</span>
              </div>
              <p className="text-xs text-zinc-500">部署后健康检查，告警/错误率/延迟</p>
            </button>
          </div>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="text-center"
        >
          <p className="text-xs text-zinc-600">
            输入故障描述或系统现象，AI 将自动调度 Metrics/Logs/Knowledge Agent 协同诊断
          </p>
        </motion.div>
      </div>
    </div>
  )
}
