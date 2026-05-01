import { useEffect, useState, useRef } from 'react'
import ReactMarkdown from 'react-markdown'

interface Props {
  content: string
}

export function StreamingText({ content }: Props) {
  const [displayed, setDisplayed] = useState('')
  const indexRef = useRef(0)

  useEffect(() => {
    if (content.length <= indexRef.current) return
    const timer = setInterval(() => {
      if (indexRef.current < content.length) {
        indexRef.current += 1 + Math.floor(Math.random() * 3)
        if (indexRef.current > content.length) indexRef.current = content.length
        setDisplayed(content.slice(0, indexRef.current))
      } else {
        clearInterval(timer)
      }
    }, 20)
    return () => clearInterval(timer)
  }, [content])

  return (
    <div className="prose prose-sm max-w-none prose-headings:text-zinc-900 prose-p:text-zinc-700 prose-strong:text-zinc-900 prose-li:text-zinc-700 prose-pre:border prose-pre:border-zinc-200 prose-pre:bg-zinc-50 prose-code:text-accent dark:prose-invert dark:prose-headings:text-zinc-100 dark:prose-p:text-zinc-200 dark:prose-strong:text-zinc-100 dark:prose-li:text-zinc-200 dark:prose-pre:border-zinc-800 dark:prose-pre:bg-zinc-950">
      <ReactMarkdown>{displayed}</ReactMarkdown>
      <span className="inline-block w-1.5 h-4 bg-accent ml-0.5 animate-typing-cursor align-middle" />
    </div>
  )
}
