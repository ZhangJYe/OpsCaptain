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
}

export function MessageBubble({ message, onOpenDetail }: Props) {
  const isUser = message.role === 'user'
  const isGoS = !isUser && isGoSMessage(message)
  const timeLabel = new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(message.timestamp)

  if (!isUser && isGoS) {
    return (
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-sky-400/15 text-[10px] font-bold text-sky-600 ring-1 ring-inset ring-sky-400/20 dark:text-sky-400">
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
      <div
        className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-[10px] font-bold ring-1 ring-inset ${
          isUser
            ? 'bg-sky-500 text-white ring-sky-400/30'
            : 'bg-sky-400/15 text-sky-600 ring-sky-400/20 dark:text-sky-400'
        }`}
      >
        {isUser ? '你' : 'OC'}
      </div>

      <div className={`min-w-0 ${isUser ? 'max-w-[75%]' : 'flex-1 max-w-[85%]'}`}>
        <div className={`mb-1.5 flex items-center gap-2 ${isUser ? 'justify-end' : ''}`}>
          <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">
            {isUser ? '你' : 'OpsCaption'}
          </span>
          <span className="text-[10px] text-zinc-400 dark:text-zinc-600">{timeLabel}</span>
        </div>

        <div
          className={`px-4 py-3 ${
            isUser
              ? 'rounded-2xl rounded-tr-sm bg-sky-500 text-white shadow-md shadow-sky-500/20'
              : 'rounded-xl rounded-bl-md border border-white/70 bg-white/80 text-zinc-800 shadow-[0_12px_34px_rgba(15,23,42,0.06)] backdrop-blur-md dark:border-white/10 dark:bg-slate-800/60 dark:text-zinc-200'
          }`}
        >
          {isUser ? (
            <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{message.content}</p>
          ) : (
            <>
              {message.executionSteps && message.executionSteps.length > 0 && (
                <ThinkingCollapse steps={message.executionSteps} onStepClick={onOpenDetail} />
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
