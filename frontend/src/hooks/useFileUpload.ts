import { useState, useCallback, useEffect, useRef } from 'react'
import { getApiBaseUrl, generateId } from '../lib/utils'

export type FileUploadStatus = 'uploading' | 'indexing' | 'ready' | 'failed'

export interface UploadedFile {
  name: string
  id: string
  size: number
  status: FileUploadStatus
}

interface UseFileUploadReturn {
  files: UploadedFile[]
  readyFiles: UploadedFile[]
  isUploading: boolean
  uploadError: string | null
  clearFiles: () => void
  removeFile: (id: string) => void
  inputId: string
  handleChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  accept: string
  multiple: boolean
}

export function useFileUpload(): UseFileUploadReturn {
  const [files, setFiles] = useState<UploadedFile[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [inputId] = useState(() => `file-upload-${generateId().slice(0, 8)}`)
  const filesRef = useRef(files)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  filesRef.current = files

  const uploadFile = useCallback(async (file: File): Promise<UploadedFile | null> => {
    const baseUrl = getApiBaseUrl()
    const formData = new FormData()
    formData.append('file', file)

    const res = await fetch(`${baseUrl}/upload`, {
      method: 'POST',
      body: formData,
    })

    if (!res.ok) {
      const text = await res.text()
      let msg = `上传失败 (${res.status})`
      try {
        const data = JSON.parse(text)
        msg = data.message || msg
      } catch { /* use default */ }
      throw new Error(msg)
    }

    const data = await res.json()
    const payload = data?.data || data
    return {
      name: payload.fileName || file.name,
      id: payload.fileId || '',
      size: payload.fileSize || file.size,
      status: payload.status === 'indexing' ? 'indexing' : (payload.status === 'failed' ? 'failed' : 'ready'),
    }
  }, [])

  const pollIndexingStatus = useCallback(async () => {
    const indexingFiles = filesRef.current.filter((f) => f.status === 'indexing')
    if (indexingFiles.length === 0) return

    const baseUrl = getApiBaseUrl()
    for (const file of indexingFiles) {
      try {
        const res = await fetch(`${baseUrl}/upload_status?file_id=${encodeURIComponent(file.id)}`)
        if (!res.ok) continue
        const data = await res.json()
        const payload = data?.data || data
        if (payload.status === 'ready' || payload.status === 'failed') {
          setFiles((curr) =>
            curr.map((f) => (f.id === file.id ? { ...f, status: payload.status as FileUploadStatus } : f))
          )
        }
      } catch { /* ignore poll errors */ }
    }
  }, [])

  useEffect(() => {
    const hasIndexing = files.some((f) => f.status === 'indexing')
    if (hasIndexing && !pollRef.current) {
      pollRef.current = setInterval(pollIndexingStatus, 2000)
    } else if (!hasIndexing && pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
    return () => {
      if (!files.some((f) => f.status === 'indexing') && pollRef.current) {
        clearInterval(pollRef.current)
        pollRef.current = null
      }
    }
  }, [files, pollIndexingStatus])

  const handleChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFiles = e.target.files
    if (!selectedFiles || selectedFiles.length === 0) return
    if (isUploading) return

    setIsUploading(true)
    setUploadError(null)

    const results: UploadedFile[] = []

    for (let i = 0; i < selectedFiles.length; i++) {
      try {
        const uploaded = await uploadFile(selectedFiles[i])
        if (uploaded) results.push(uploaded)
      } catch (err: any) {
        setUploadError(err?.message || '上传失败')
        break
      }
    }

    setFiles((prev) => [...prev, ...results])
    setIsUploading(false)

    e.target.value = ''
  }, [uploadFile, isUploading])

  const removeFile = useCallback((id: string) => {
    setFiles((prev) => prev.filter((f) => f.id !== id))
  }, [])

  const clearFiles = useCallback(() => {
    setFiles([])
    setUploadError(null)
  }, [])

  const readyFiles = files.filter((f) => f.status === 'ready')

  return {
    files,
    readyFiles,
    isUploading,
    uploadError,
    clearFiles,
    removeFile,
    inputId,
    handleChange,
    accept: '.md,.txt,.pdf,.doc,.docx,.csv,.json,.yaml,.yml',
    multiple: true,
  }
}
