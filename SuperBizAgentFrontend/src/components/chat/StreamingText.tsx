import ReactMarkdown from 'react-markdown'

interface Props {
  content: string
}

export function StreamingText({ content }: Props) {
  return (
    <div className="prose prose-sm max-w-none prose-headings:text-zinc-900 prose-p:text-zinc-700 prose-strong:text-zinc-900 prose-li:text-zinc-700 prose-pre:border prose-pre:border-zinc-200 prose-pre:bg-zinc-50 prose-code:text-accent dark:prose-invert dark:prose-headings:text-zinc-100 dark:prose-p:text-zinc-200 dark:prose-strong:text-zinc-100 dark:prose-li:text-zinc-200 dark:prose-pre:border-zinc-800 dark:prose-pre:bg-zinc-950">
      <ReactMarkdown>{content}</ReactMarkdown>
      <span className="inline-block w-1.5 h-4 bg-accent ml-0.5 animate-typing-cursor align-middle" />
    </div>
  )
}
