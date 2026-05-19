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
    <div className="rounded-xl border border-zinc-200/80 border-l-2 border-l-accent bg-gradient-to-br from-white via-white to-accent/[0.03] px-4 py-3 shadow-sm shadow-zinc-900/[0.03] dark:border-zinc-800/60 dark:border-l-accent dark:from-zinc-900 dark:via-zinc-900 dark:to-accent/[0.06]">
      {/* Header */}
      <div className="mb-2 flex items-center gap-2 text-[11px] font-medium text-accent">
        <Brain size={13} />
        <span>GoS Belief Report</span>
        {isStreaming ? (
          <span className="ml-auto flex items-center gap-1 text-[10px] text-zinc-400">
            <Loader2 size={10} className="animate-spin" />
            推理中...
          </span>
        ) : isDone ? (
          <span className="ml-auto rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] text-emerald-500">
            完成
          </span>
        ) : null}
      </div>

      {/* Thinking chain */}
      {hasSteps && (
        <ThinkingCollapse steps={executionSteps} isStreaming={isStreaming} defaultOpen={isStreaming} />
      )}

      {/* Divider */}
      {hasSteps && body && (
        <div className="my-2 border-t border-zinc-100 dark:border-zinc-800/60" />
      )}

      {/* Report body */}
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
  )
}
