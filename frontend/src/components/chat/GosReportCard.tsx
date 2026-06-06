import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkFixHeadings, { normalizeLooseMarkdown } from '../../lib/remarkFixHeadings'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import type { ChatMessage } from '../../types/chat'
import { Brain, Loader2 } from 'lucide-react'
import { ENGINE_VIEW_MODEL } from '../../lib/engineViewModel'

interface Props {
  message?: ChatMessage
  steps?: ThinkingStep[]
  content?: string
  isStreaming?: boolean
}

export function GosReportCard({ message, steps, content, isStreaming }: Props) {
  const executionSteps = steps || message?.executionSteps || []
  const body = content || message?.content || ''
  const hasSteps = executionSteps.length > 0
  const isDone = !isStreaming && executionSteps.length > 0 && executionSteps.every((s) => s.status === 'done' || s.status === 'error')
  const view = ENGINE_VIEW_MODEL.gos_engine
  const hypothesis = executionSteps.find((s) => s.id === 'gos:hypothesis')
  const evidence = executionSteps.find((s) => s.id === 'gos:evidence')
  const confidence = executionSteps.find((s) => s.id === 'gos:confidence')
  const beliefItems = [
    { label: 'Hypothesis', value: hypothesis?.detail || '候选假设收集中' },
    { label: 'Evidence', value: evidence?.meta?.[0] || evidence?.detail || '证据挂载中' },
    { label: 'Confidence', value: confidence?.detail || '置信度校准中' },
  ]

  return (
    <div className="relative">
      {isStreaming && <div aria-hidden="true" className="glow-frame rounded-[22px] rounded-bl-[6px]" />}

      <div className="relative rounded-[22px] rounded-bl-[6px] border border-white/60 border-l-2 border-l-amber-400 bg-white/70 px-4 py-3 backdrop-blur-2xl dark:border-white/10 dark:border-l-amber-400 dark:bg-slate-800/50">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold text-amber-600 dark:text-amber-400">
          <span className={`flex h-6 w-6 items-center justify-center rounded-lg ring-1 ${view.sidebar.icon}`}>
            <Brain size={13} />
          </span>
          <span>{view.resultTitle}</span>
          {isStreaming ? (
            <span className="ml-auto flex items-center gap-1 text-[10px] text-sky-500">
              <Loader2 size={10} className="animate-spin" />
              推理中...
            </span>
          ) : isDone ? (
            <span className="ml-auto flex items-center gap-1 text-[10px]">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
              <span className="text-emerald-600 dark:text-emerald-400">完成</span>
            </span>
          ) : null}
        </div>

        {hasSteps && (
          <div className="mb-3 grid gap-2 sm:grid-cols-3">
            {beliefItems.map((item) => (
              <div key={item.label} className="rounded-xl border border-amber-200/50 bg-amber-50/35 px-3 py-2 dark:border-amber-500/15 dark:bg-amber-500/10">
                <div className="flex items-center gap-1.5">
                  <span className={`h-1.5 w-1.5 rounded-full ${view.sidebar.dot}`} />
                  <span className="text-[10px] font-semibold text-amber-600 dark:text-amber-300">{item.label}</span>
                </div>
                <p className="mt-1 line-clamp-2 text-[11px] leading-5 text-zinc-600 dark:text-zinc-300">{item.value}</p>
              </div>
            ))}
          </div>
        )}

        {hasSteps && (
          <ThinkingCollapse steps={executionSteps} isStreaming={isStreaming} defaultOpen={isStreaming} />
        )}

        {hasSteps && body && (
          <div className="my-2 border-t border-white/40 dark:border-white/5" />
        )}

        {body && (
          <div className="prose-chat">
            <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
              {normalizeLooseMarkdown(body)}
            </ReactMarkdown>
          </div>
        )}
      </div>
    </div>
  )
}
