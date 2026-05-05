import { useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkFixHeadings from '../../lib/remarkFixHeadings'

interface Props {
  content: string
}

export function StreamingText({ content }: Props) {
  const [visibleContent, setVisibleContent] = useState('')
  const targetRef = useRef(content)
  const visibleRef = useRef('')
  const timerRef = useRef<number | null>(null)

  useEffect(() => {
    visibleRef.current = visibleContent
  }, [visibleContent])

  useEffect(() => {
    targetRef.current = content
    if (content.length < visibleRef.current.length) {
      visibleRef.current = content
      setVisibleContent(content)
      return
    }
    if (timerRef.current !== null || visibleRef.current.length >= content.length) {
      return
    }

    const tick = () => {
      const target = targetRef.current
      const current = visibleRef.current
      if (current.length >= target.length) {
        timerRef.current = null
        return
      }
      const remaining = target.length - current.length
      const step = remaining > 120 ? 10 : remaining > 48 ? 6 : remaining > 16 ? 3 : 1
      const next = target.slice(0, Math.min(target.length, current.length + step))
      visibleRef.current = next
      setVisibleContent(next)
      if (next.length < targetRef.current.length) {
        timerRef.current = window.setTimeout(tick, 16)
      } else {
        timerRef.current = null
      }
    }

    timerRef.current = window.setTimeout(tick, 16)
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }
  }, [content])

  return (
    <div className="prose-chat">
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkFixHeadings]}>
        {visibleContent}
      </ReactMarkdown>
      <span className="inline-block w-1.5 h-4 bg-accent ml-0.5 animate-typing-cursor align-middle" />
    </div>
  )
}
