import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ChatMessage } from '../../types/chat'
import { ThinkingCollapse } from '../agent/ThinkingCollapse'

interface Props {
  message: ChatMessage
}

export function MessageBubble({ message }: Props) {
  const isUser = message.role === 'user'
  const timeLabel = new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(message.timestamp)

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
              : 'border border-zinc-200/80 bg-white/90 text-zinc-800 shadow-sm dark:border-zinc-800/60 dark:bg-zinc-900/70 dark:text-zinc-200'
          }`}
        >
          {isUser ? (
            <p className="whitespace-pre-wrap break-words text-sm leading-7">{message.content}</p>
          ) : (
            <>
              {message.executionSteps && message.executionSteps.length > 0 && (
                <ThinkingCollapse steps={message.executionSteps} defaultOpen />
              )}
              <div className="prose prose-sm max-w-none leading-7 prose-headings:text-zinc-900 prose-p:text-zinc-700 prose-strong:text-zinc-900 prose-li:text-zinc-700 prose-pre:border prose-pre:border-zinc-200 prose-pre:bg-zinc-50 prose-pre:rounded-xl prose-code:text-accent prose-code:before:content-none prose-code:after:content-none prose-a:text-accent dark:prose-invert dark:prose-headings:text-white dark:prose-p:text-zinc-300 dark:prose-strong:text-white dark:prose-li:text-zinc-300 dark:prose-pre:border-zinc-800 dark:prose-pre:bg-zinc-950">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
