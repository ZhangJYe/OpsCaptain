import { useState, useCallback } from 'react'
import type { ChatMessage, ChatMode } from '../types/chat'
import { getApiBaseUrl, generateId } from '../lib/utils'

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [streamingContent, setStreamingContent] = useState('')
  const [mode, setMode] = useState<ChatMode>('quick')
  const [sessionId] = useState(() => generateId())
  const [abortCtrl, setAbortCtrl] = useState<AbortController | null>(null)

  const send = useCallback(
    async (query: string) => {
      if (!query.trim() || isLoading) return

      const userMsg: ChatMessage = {
        id: generateId(),
        role: 'user',
        content: query,
        timestamp: Date.now(),
      }
      setMessages((prev) => [...prev, userMsg])
      setIsLoading(true)
      setStreamingContent('')

      const baseUrl = getApiBaseUrl()

      if (mode === 'quick') {
        try {
          const res = await fetch(`${baseUrl}/chat`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ query, session_id: sessionId }),
          })
          const data = await res.json()
          const assistantMsg: ChatMessage = {
            id: generateId(),
            role: 'assistant',
            content: data.content || data.message || '无响应',
            timestamp: Date.now(),
          }
          setMessages((prev) => [...prev, assistantMsg])
        } catch (err: any) {
          setMessages((prev) => [
            ...prev,
            { id: generateId(), role: 'assistant', content: `请求失败: ${err.message}`, timestamp: Date.now() },
          ])
        } finally {
          setIsLoading(false)
        }
      } else {
        const ctrl = new AbortController()
        setAbortCtrl(ctrl)

        try {
          const res = await fetch(`${baseUrl}/chat_stream`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ query, session_id: sessionId }),
            signal: ctrl.signal,
          })
          const reader = res.body?.getReader()
          if (!reader) throw new Error('No response body')

          const decoder = new TextDecoder()
          let fullContent = ''

          while (true) {
            const { done, value } = await reader.read()
            if (done) break
            const chunk = decoder.decode(value, { stream: true })
            const lines = chunk.split('\n')
            for (const line of lines) {
              if (line.startsWith('data: ')) {
                try {
                  const json = JSON.parse(line.slice(6))
                  const text = json.content || json.choices?.[0]?.delta?.content || ''
                  fullContent += text
                  setStreamingContent(fullContent)
                } catch {
                  // skip non-JSON lines
                }
              }
            }
          }

          setMessages((prev) => [
            ...prev,
            { id: generateId(), role: 'assistant', content: fullContent, timestamp: Date.now() },
          ])
        } catch (err: any) {
          if (err.name !== 'AbortError') {
            setMessages((prev) => [
              ...prev,
              { id: generateId(), role: 'assistant', content: `流式请求失败: ${err.message}`, timestamp: Date.now() },
            ])
          }
        } finally {
          setIsLoading(false)
          setStreamingContent('')
          setAbortCtrl(null)
        }
      }
    },
    [isLoading, mode, sessionId]
  )

  const stop = useCallback(() => {
    abortCtrl?.abort()
    setIsLoading(false)
    setAbortCtrl(null)
  }, [abortCtrl])

  const clear = useCallback(() => {
    setMessages([])
    setStreamingContent('')
  }, [])

  return { messages, streamingContent, isLoading, mode, sessionId, send, stop, clear, setMode, setMessages }
}
