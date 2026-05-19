import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkFixHeadings from '../../lib/remarkFixHeadings'
import type { ChatMessage } from '../../types/chat'
import type { DetailItem } from '../workbench/DetailPanel'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'
import { GosReportCard } from './GosReportCard'
import { isGoSMessage } from '../../lib/utils'

interface Props {
  message: ChatMessage
  onOpenDetail?: (item: DetailItem) => void
  hideSteps?: boolean
}

export function MessageBubble({ message, hideSteps }: Props) {
  const isUser = message.role === 'user'
  const isGoS = !isUser && isGoSMessage(message)
  const timeLabel = new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(message.timestamp)

  if (!isUser && isGoS) {
    return (
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent/10 text-xs font-semibold text-accent ring-1 ring-inset ring-accent/20">
          OC
        </div>
        <div className="min-w-0 flex-1 max-w-[85%]">
          <div className="mb-1.5 flex items-center gap-2">
            <span className="text-[11px] font-medium text-zinc-500">OpsCaption</span>
            <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{timeLabel}</span>
          </div>
          <GosReportCard message={message} />
        </div>
      </div>
    )
  }

  return (
    <div className={`flex items-start gap-3 ${isUser ? 'flex-row-reverse' : ''}`}>
      {/* Avatar */}
      <div
        className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold ring-1 ring-inset ${
          isUser
            ? 'bg-zinc-100 text-zinc-600 ring-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-700'
            : 'bg-accent/10 text-accent ring-accent/20'
        }`}
      >
        {isUser ? '你' : 'OC'}
      </div>

      {/* Content */}
      <div className={`min-w-0 ${isUser ? 'max-w-[75%]' : 'flex-1 max-w-[85%]'}`}>
        {/* Meta line */}
        <div className={`mb-1.5 flex items-center gap-2 ${isUser ? 'justify-end' : ''}`}>
          <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">
            {isUser ? '你' : 'OpsCaption'}
          </span>
          <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{timeLabel}</span>
        </div>

        {/* Bubble */}
        <div
          className={`rounded-2xl px-4 py-3 ${
            isUser
              ? 'bg-accent text-white shadow-sm shadow-accent/10'
              : 'border border-zinc-200/80 bg-white/95 text-zinc-800 shadow-sm shadow-zinc-900/[0.03] dark:border-zinc-800/60 dark:bg-zinc-900/80 dark:text-zinc-200'
          }`}
        >
          {isUser ? (
            <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{message.content}</p>
          ) : (
            <>
              {!hideSteps && message.executionSteps && message.executionSteps.length > 0 && (
                <ThinkingCollapse steps={message.executionSteps} defaultOpen />
              )}
              <div className="prose-chat">
                <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
                  {message.content}
                </ReactMarkdown>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
