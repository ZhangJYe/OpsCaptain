import { useCallback, useState } from 'react'
import type { ChatMessage, ChatMode, ChatSession } from '../types/chat'
import { buildSkillAwareQuery, generateId, getApiBaseUrl } from '../lib/utils'

interface SendOptions {
  selectedSkillIds?: string[]
}

function parseJsonSafe(raw: string): any {
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function normalizeResponsePayload(data: any): any {
  if (data && typeof data === 'object' && 'data' in data && data.data) {
    return data.data
  }
  return data
}

function extractAnswer(payload: any): string {
  const content = payload?.answer || payload?.content || payload?.message || ''
  return String(content || '').trim() || '无响应'
}

function parseSSEBlock(block: string): { event: string; data: string } {
  let event = 'message'
  const dataLines: string[] = []

  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim() || 'message'
      continue
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }

  return {
    event,
    data: dataLines.join('\n'),
  }
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [streamingContent, setStreamingContent] = useState('')
  const [streamingThoughts, setStreamingThoughts] = useState<string[]>([])
  const [mode, setMode] = useState<ChatMode>('quick')
  const [sessionId, setSessionId] = useState(() => generateId())
  const [abortCtrl, setAbortCtrl] = useState<AbortController | null>(null)

  const send = useCallback(
    async (query: string, options: SendOptions = {}) => {
      const trimmed = String(query || '').trim()
      if (!trimmed || isLoading) return

      const requestQuery = buildSkillAwareQuery(trimmed, options.selectedSkillIds || [])
      const userMsg: ChatMessage = {
        id: generateId(),
        role: 'user',
        content: trimmed,
        timestamp: Date.now(),
      }

      setMessages((prev) => [...prev, userMsg])
      setIsLoading(true)
      setStreamingContent('')
      setStreamingThoughts([])

      const baseUrl = getApiBaseUrl()

      if (mode === 'quick') {
        try {
          const res = await fetch(`${baseUrl}/chat`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ Id: sessionId, Question: requestQuery }),
          })
          const data = await res.json()
          const payload = normalizeResponsePayload(data)
          if (!res.ok) {
            throw new Error(String(data?.message || `HTTP ${res.status}`))
          }
          const assistantMsg: ChatMessage = {
            id: generateId(),
            role: 'assistant',
            content: extractAnswer(payload),
            timestamp: Date.now(),
          }
          setMessages((prev) => [...prev, assistantMsg])
        } catch (err: any) {
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: 'assistant',
              content: `请求失败: ${err?.message || '未知错误'}`,
              timestamp: Date.now(),
            },
          ])
        } finally {
          setIsLoading(false)
        }
        return
      }

      const ctrl = new AbortController()
      setAbortCtrl(ctrl)
      let partialContent = ''

      try {
        const res = await fetch(`${baseUrl}/chat_stream`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ Id: sessionId, Question: requestQuery }),
          signal: ctrl.signal,
        })

        if (!res.ok) {
          const text = await res.text()
          const maybeJson = parseJsonSafe(text)
          throw new Error(String(maybeJson?.message || text || `HTTP ${res.status}`))
        }

        const reader = res.body?.getReader()
        if (!reader) throw new Error('No response body')

        const decoder = new TextDecoder()
        let buffer = ''
        let fullContent = ''

        while (true) {
          const { done, value } = await reader.read()
          if (value) {
            buffer += decoder.decode(value, { stream: !done })
          }

          let separatorIndex = buffer.indexOf('\n\n')
          while (separatorIndex >= 0) {
            const block = buffer.slice(0, separatorIndex)
            buffer = buffer.slice(separatorIndex + 2)
            const { event, data } = parseSSEBlock(block)

            if (event === 'message') {
              fullContent += data
              partialContent = fullContent
              setStreamingContent(fullContent)
            } else if (event === 'thought') {
              const thought = data.trim()
              if (thought) {
                setStreamingThoughts((prev) => (prev.includes(thought) ? prev : [...prev, thought]))
              }
            } else if (event === 'error') {
              throw new Error(data || '流式请求失败')
            }

            separatorIndex = buffer.indexOf('\n\n')
          }

          if (done) {
            break
          }
        }

        if (buffer.trim()) {
          const { event, data } = parseSSEBlock(buffer)
          if (event === 'message') {
            fullContent += data
            partialContent = fullContent
            setStreamingContent(fullContent)
          }
        }

        if (fullContent.trim()) {
          setMessages((prev) => [
            ...prev,
            { id: generateId(), role: 'assistant', content: fullContent, timestamp: Date.now() },
          ])
        }
      } catch (err: any) {
        const isAbort = err?.name === 'AbortError'
        setMessages((prev) => {
          if (partialContent.trim()) {
            return [
              ...prev,
              {
                id: generateId(),
                role: 'assistant',
                content: partialContent,
                timestamp: Date.now(),
              },
            ]
          }
          if (isAbort) {
            return prev
          }
          return [
            ...prev,
            {
              id: generateId(),
              role: 'assistant',
              content: `流式请求失败: ${err?.message || '未知错误'}`,
              timestamp: Date.now(),
            },
          ]
        })
      } finally {
        setIsLoading(false)
        setStreamingContent('')
        setStreamingThoughts([])
        setAbortCtrl(null)
      }
    },
    [isLoading, mode, sessionId]
  )

  const stop = useCallback(() => {
    abortCtrl?.abort()
    setIsLoading(false)
    setAbortCtrl(null)
  }, [abortCtrl])

  const newSession = useCallback(() => {
    if (isLoading) {
      return false
    }
    setMessages([])
    setStreamingContent('')
    setStreamingThoughts([])
    setMode('quick')
    setSessionId(generateId())
    return true
  }, [isLoading])

  const loadSession = useCallback(
    (session: ChatSession) => {
      if (isLoading || !session) {
        return false
      }
      setSessionId(session.id)
      setMessages(Array.isArray(session.messages) ? session.messages : [])
      setMode(session.mode === 'stream' ? 'stream' : 'quick')
      setStreamingContent('')
      setStreamingThoughts([])
      return true
    },
    [isLoading]
  )

  return {
    messages,
    streamingContent,
    streamingThoughts,
    isLoading,
    mode,
    sessionId,
    send,
    stop,
    newSession,
    loadSession,
    setMode,
    setMessages,
  }
}
