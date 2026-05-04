import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface Props {
  content: string
}

export function StreamingText({ content }: Props) {
  return (
    <div className="prose prose-sm max-w-none leading-relaxed prose-headings:mt-4 prose-headings:mb-2 prose-p:my-2 prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5 prose-pre:my-3 prose-pre:p-3 prose-pre:overflow-x-auto prose-pre:rounded-xl prose-pre:border prose-pre:border-zinc-200 prose-pre:bg-zinc-50 prose-code:text-[13px] prose-code:font-normal prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded-md prose-code:bg-zinc-100 prose-code:before:content-none prose-code:after:content-none prose-pre:prose-code:bg-transparent prose-pre:prose-code:p-0 prose-a:text-accent prose-a:no-underline hover:prose-a:underline prose-headings:text-zinc-900 prose-p:text-zinc-700 prose-strong:text-zinc-900 prose-li:text-zinc-700 prose-code:text-accent dark:prose-invert dark:prose-headings:text-white dark:prose-p:text-zinc-300 dark:prose-strong:text-white dark:prose-li:text-zinc-300 dark:prose-pre:border-zinc-800 dark:prose-pre:bg-zinc-950/80 dark:prose-code:bg-zinc-800/80 dark:prose-code:text-accent">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
      <span className="inline-block w-1.5 h-4 bg-accent ml-0.5 animate-typing-cursor align-middle" />
    </div>
  )
}
