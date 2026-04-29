import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ChatMessage } from '../../types/chat'

interface Props {
  message: ChatMessage
}

export function MessageBubble({ message }: Props) {
  const isUser = message.role === 'user'

  return (
    <div className={`flex items-start gap-3 ${isUser ? 'flex-row-reverse' : ''}`}>
      <div
        className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 mt-1 ${
          isUser ? 'bg-zinc-700' : 'bg-accent/20'
        }`}
      >
        <span className={`text-xs font-bold ${isUser ? 'text-zinc-300' : 'text-accent'}`}>
          {isUser ? '你' : 'AI'}
        </span>
      </div>

      <div
        className={`flex-1 min-w-0 rounded-2xl px-4 py-3 ${
          isUser ? 'bg-accent/10 border border-accent/20' : 'glass'
        }`}
      >
        {isUser ? (
          <p className="text-sm whitespace-pre-wrap break-words">{message.content}</p>
        ) : (
          <div className="prose prose-invert prose-sm max-w-none prose-pre:bg-zinc-950 prose-pre:border prose-pre:border-zinc-800 prose-code:text-accent">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
          </div>
        )}
      </div>
    </div>
  )
}
