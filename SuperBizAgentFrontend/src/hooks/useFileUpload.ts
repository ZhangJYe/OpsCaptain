import { useState, useCallback } from 'react'
import { getApiBaseUrl, generateId } from '../lib/utils'

export interface UploadedFile {
  name: string
  id: string
  size: number
}

interface UseFileUploadReturn {
  files: UploadedFile[]
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
    }
  }, [])

  const handleChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFiles = e.target.files
    if (!selectedFiles || selectedFiles.length === 0) return

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

    // Reset input so same file can be re-selected
    e.target.value = ''
  }, [uploadFile])

  const removeFile = useCallback((id: string) => {
    setFiles((prev) => prev.filter((f) => f.id !== id))
  }, [])

  const clearFiles = useCallback(() => {
    setFiles([])
    setUploadError(null)
  }, [])

  return {
    files,
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
