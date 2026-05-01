import { motion } from 'framer-motion'
import { ArrowRight, RefreshCw, FileSearch, BookOpen, BarChart3, Shield } from 'lucide-react'

export interface Suggestion {
  label: string
  query: string
  icon?: 'metrics' | 'logs' | 'knowledge' | 'action' | 'retry'
}

interface Props {
  suggestions: Suggestion[]
  onSelect: (query: string) => void
}

const icons = {
  metrics: BarChart3,
  logs: FileSearch,
  knowledge: BookOpen,
  action: Shield,
  retry: RefreshCw,
}

export function SuggestionChips({ suggestions, onSelect }: Props) {
  if (suggestions.length === 0) return null

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay: 0.1 }}
      className="flex flex-wrap items-center gap-2"
    >
      <span className="text-[10px] text-zinc-400 dark:text-zinc-600">继续排查：</span>
      {suggestions.map((s, i) => {
        const Icon = s.icon ? icons[s.icon] : ArrowRight
        return (
          <button
            key={i}
            onClick={() => onSelect(s.query)}
            className="inline-flex items-center gap-1.5 rounded-lg border border-zinc-200/80 bg-white/70 px-2.5 py-1.5 text-xs text-zinc-600 transition-all hover:border-accent/30 hover:bg-accent/5 hover:text-accent dark:border-zinc-800/60 dark:bg-zinc-900/50 dark:text-zinc-400 dark:hover:border-accent/30 dark:hover:text-accent"
          >
            <Icon size={12} />
            {s.label}
          </button>
        )
      })}
    </motion.div>
  )
}

/** Generate contextual suggestions based on the last assistant response */
export function generateSuggestions(responseContent: string, mode: string): Suggestion[] {
  const content = responseContent.toLowerCase()
  const suggestions: Suggestion[] = []

  if (content.includes('延迟') || content.includes('latency') || content.includes('timeout')) {
    suggestions.push({ label: '查看详细指标', query: '请展示相关的延迟和错误率指标趋势', icon: 'metrics' })
    suggestions.push({ label: '分析日志特征', query: '请分析超时相关的日志，找出高频错误模式', icon: 'logs' })
  }

  if (content.includes('错误') || content.includes('error') || content.includes('失败')) {
    suggestions.push({ label: '检索历史案例', query: '请检索与当前错误模式相似的历史案例', icon: 'knowledge' })
    suggestions.push({ label: '生成处置方案', query: '请基于当前分析给出回滚、限流和验证步骤', icon: 'action' })
  }

  if (content.includes('回滚') || content.includes('rollback') || content.includes('处置')) {
    suggestions.push({ label: '检查变更窗口', query: '请检查最近的发布和配置变更记录', icon: 'logs' })
    suggestions.push({ label: '验证恢复状态', query: '请检查回滚后各服务的健康状态和指标恢复情况', icon: 'metrics' })
  }

  if (content.includes('数据库') || content.includes('database') || content.includes('连接池')) {
    suggestions.push({ label: '检查连接池', query: '请分析数据库连接池的使用情况和等待队列', icon: 'metrics' })
    suggestions.push({ label: '查看慢查询', query: '请检索数据库慢查询日志，定位具体SQL', icon: 'logs' })
  }

  // Always add fallback suggestions
  if (suggestions.length < 2) {
    suggestions.push({ label: '深入分析', query: '请更深入地分析当前问题，补充更多证据', icon: 'retry' })
    suggestions.push({ label: '生成报告', query: '请将以上诊断整理为结构化的事故报告', icon: 'action' })
  }

  // Limit to 4
  return suggestions.slice(0, 4)
}
