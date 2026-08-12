export type KnowledgeDocumentStatus = 'indexing' | 'ready' | 'failed'

export interface KnowledgeDocument {
  file_id: string
  file_name: string
  file_size: number
  mime_type: string
  status: KnowledgeDocumentStatus
  uploaded_at: string
  version: number
}

export function filterKnowledgeDocuments(
  documents: KnowledgeDocument[],
  query: string,
  status: KnowledgeDocumentStatus | 'all',
): KnowledgeDocument[] {
  const normalized = query.trim().toLocaleLowerCase()
  return documents.filter((document) => (
    (status === 'all' || document.status === status)
    && (!normalized || document.file_name.toLocaleLowerCase().includes(normalized))
  ))
}

export function knowledgeStatusMeta(status: KnowledgeDocumentStatus): { label: string; className: string } {
  switch (status) {
    case 'ready':
      return { label: '已就绪', className: 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-500/20' }
    case 'failed':
      return { label: '索引失败', className: 'bg-rose-50 text-rose-700 ring-rose-200 dark:bg-rose-500/10 dark:text-rose-300 dark:ring-rose-500/20' }
    default:
      return { label: '索引中', className: 'bg-sky-50 text-sky-700 ring-sky-200 dark:bg-sky-500/10 dark:text-sky-300 dark:ring-sky-500/20' }
  }
}

export function formatKnowledgeFileSize(size: number): string {
  if (size < 1024 * 1024) return `${Math.max(1, Math.round(size / 1024))} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

export function formatKnowledgeDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  return date.toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
