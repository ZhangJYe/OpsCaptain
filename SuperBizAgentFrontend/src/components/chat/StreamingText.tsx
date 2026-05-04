import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkFixHeadings from '../../lib/remarkFixHeadings'

interface Props {
  content: string
}

export function StreamingText({ content }: Props) {
  return (
    <div className="prose-chat">
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
        {content}
      </ReactMarkdown>
      <span className="inline-block w-1.5 h-4 bg-accent ml-0.5 animate-typing-cursor align-middle" />
    </div>
  )
}
