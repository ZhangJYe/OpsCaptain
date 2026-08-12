import { useCallback, useEffect, useState } from 'react'
import { getApiBaseUrl } from '../lib/utils'
import type { KnowledgeDocument, KnowledgeDocumentStatus } from '../lib/knowledgeDocuments'

const ACCEPT = '.md,.txt,.pdf,.doc,.docx,.csv,.json,.yaml,.yml'

function unwrap(payload: any) {
  return payload?.data || payload
}

async function responseMessage(response: Response): Promise<string> {
  const text = await response.text()
  try {
    const payload = unwrap(JSON.parse(text))
    return String(payload?.message || `请求失败 (${response.status})`)
  } catch {
    return text || `请求失败 (${response.status})`
  }
}

export function useKnowledgeDocuments() {
  const [documents, setDocuments] = useState<KnowledgeDocument[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isUploading, setIsUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await fetch(`${getApiBaseUrl()}/knowledge_documents`)
      if (!response.ok) throw new Error(await responseMessage(response))
      const payload = unwrap(await response.json())
      setDocuments(Array.isArray(payload?.items) ? payload.items : [])
      setError(null)
    } catch (reason: any) {
      setError(reason?.message || '无法加载知识资料')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  useEffect(() => {
    if (!documents.some((document) => document.status === 'indexing')) return undefined
    const timer = window.setInterval(() => void refresh(), 2000)
    return () => window.clearInterval(timer)
  }, [documents, refresh])

  const upload = useCallback(async (files: FileList | File[]) => {
    if (!files.length || isUploading) return
    setIsUploading(true)
    setError(null)
    try {
      for (const file of Array.from(files)) {
        const body = new FormData()
        body.append('file', file)
        const response = await fetch(`${getApiBaseUrl()}/upload`, { method: 'POST', body })
        if (!response.ok) throw new Error(await responseMessage(response))
      }
      await refresh()
    } catch (reason: any) {
      setError(reason?.message || '上传资料失败')
    } finally {
      setIsUploading(false)
    }
  }, [isUploading, refresh])

  const remove = useCallback(async (fileID: string) => {
    setError(null)
    const response = await fetch(`${getApiBaseUrl()}/knowledge_documents/${encodeURIComponent(fileID)}`, { method: 'DELETE' })
    if (!response.ok) {
      const message = await responseMessage(response)
      setError(message)
      throw new Error(message)
    }
    await refresh()
  }, [refresh])

  const reindex = useCallback(async (fileID: string) => {
    setError(null)
    const response = await fetch(`${getApiBaseUrl()}/knowledge_documents/${encodeURIComponent(fileID)}/reindex`, { method: 'POST' })
    if (!response.ok) {
      const message = await responseMessage(response)
      setError(message)
      throw new Error(message)
    }
    const payload = unwrap(await response.json())
    const updated = payload?.item as KnowledgeDocument | undefined
    if (updated) {
      setDocuments((current) => current.map((document) => document.file_id === updated.file_id ? updated : document))
    }
  }, [])

  return { documents, isLoading, isUploading, error, refresh, upload, remove, reindex, accept: ACCEPT }
}
