import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkFixHeadings from '../../lib/remarkFixHeadings'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import type { ThinkingStep } from '../agent/ThinkingCollapse'
import type { ChatMessage } from '../../types/chat'
import { Brain, Loader2 } from 'lucide-react'

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

  return (
    <div className="relative">
      {isStreaming && <div aria-hidden="true" className="glow-frame rounded-[22px] rounded-bl-[6px]" />}

      <div className="relative rounded-[22px] rounded-bl-[6px] border border-white/60 border-l-2 border-l-amber-400 bg-white/70 px-4 py-3 backdrop-blur-2xl dark:border-white/10 dark:border-l-amber-400 dark:bg-slate-800/50">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold text-amber-600 dark:text-amber-400">
          <Brain size={13} />
          <span>GoS Belief Report</span>
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
          <ThinkingCollapse steps={executionSteps} isStreaming={isStreaming} defaultOpen={isStreaming} />
        )}

        {hasSteps && body && (
          <div className="my-2 border-t border-white/40 dark:border-white/5" />
        )}

        {body && (
          <div className="prose-chat">
            {isStreaming && !message ? (
              <span>{body}</span>
            ) : (
              <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
                {body}
              </ReactMarkdown>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
