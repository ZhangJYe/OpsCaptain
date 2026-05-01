import { useState, useCallback, useRef } from 'react'
import { getApiBaseUrl } from '../lib/utils'

export interface UploadedFile {
  name: string
  id: string
  size: number
}

interface UseFileUploadReturn {
  files: UploadedFile[]
  isUploading: boolean
  uploadError: string | null
  openFilePicker: () => void
  removeFile: (id: string) => void
  clearFiles: () => void
  fileInputProps: {
    ref: React.RefObject<HTMLInputElement>
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
    accept: string
    multiple: boolean
    style: React.CSSProperties
  }
}

export function useFileUpload(): UseFileUploadReturn {
  const [files, setFiles] = useState<UploadedFile[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)

  const openFilePicker = useCallback(() => {
    inputRef.current?.click()
  }, [])

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

  const handleFileChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
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
    if (inputRef.current) {
      inputRef.current.value = ''
    }
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
    openFilePicker,
    removeFile,
    clearFiles,
    fileInputProps: {
      ref: inputRef,
      onChange: handleFileChange,
      accept: '.md,.txt,.pdf,.doc,.docx,.csv,.json,.yaml,.yml',
      multiple: true,
      style: { display: 'none' },
    },
  }
}
