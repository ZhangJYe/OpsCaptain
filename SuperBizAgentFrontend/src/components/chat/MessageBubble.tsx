import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ChatMessage } from '../../types/chat'

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
      <div
        className={`mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl border ${
          isUser ? 'border-zinc-700 bg-zinc-800/90' : 'border-accent/20 bg-accent/10'
        }`}
      >
        <span className={`text-xs font-bold ${isUser ? 'text-zinc-300' : 'text-accent'}`}>
          {isUser ? '你' : 'AI'}
        </span>
      </div>

      <div className={`min-w-0 ${isUser ? 'max-w-[min(75%,42rem)]' : 'flex-1'}`}>
        <div className={`mb-2 flex items-center gap-2 text-[11px] text-zinc-500 ${isUser ? 'justify-end' : ''}`}>
          <span className="font-medium text-zinc-300">{isUser ? '你' : 'OpsCaption'}</span>
          <span>{timeLabel}</span>
        </div>
        <div
          className={`rounded-3xl px-4 py-3 ${
            isUser
              ? 'border border-accent/18 bg-accent/10 text-zinc-100'
              : 'border border-zinc-800/80 bg-zinc-900/62'
          }`}
        >
        {isUser ? (
          <p className="whitespace-pre-wrap break-words text-sm leading-7">{message.content}</p>
        ) : (
          <div className="prose prose-invert prose-sm max-w-none leading-7 prose-headings:text-zinc-100 prose-p:text-zinc-200 prose-strong:text-zinc-100 prose-li:text-zinc-200 prose-pre:border prose-pre:border-zinc-800 prose-pre:bg-zinc-950 prose-code:text-accent prose-a:text-accent">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
          </div>
        )}
        </div>
      </div>
    </div>
  )
}
