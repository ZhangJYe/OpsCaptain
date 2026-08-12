import { useMemo, useRef, useState } from 'react'
import { AlertCircle, CheckCircle2, FileText, FolderOpen, Loader2, RefreshCw, Search, Trash2, Upload, X } from 'lucide-react'
import { useKnowledgeDocuments } from '../../hooks/useKnowledgeDocuments'
import {
  filterKnowledgeDocuments,
  formatKnowledgeDate,
  formatKnowledgeFileSize,
  knowledgeStatusMeta,
  type KnowledgeDocument,
  type KnowledgeDocumentStatus,
} from '../../lib/knowledgeDocuments'

const statusOptions: Array<{ value: KnowledgeDocumentStatus | 'all'; label: string }> = [
  { value: 'all', label: '全部资料' },
  { value: 'ready', label: '已就绪' },
  { value: 'indexing', label: '索引中' },
  { value: 'failed', label: '索引失败' },
]

function StatusIcon({ status }: { status: KnowledgeDocumentStatus }) {
  if (status === 'ready') return <CheckCircle2 size={15} />
  if (status === 'failed') return <AlertCircle size={15} />
  return <Loader2 size={15} className="animate-spin" />
}

export function KnowledgeBaseView() {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { documents, isLoading, isUploading, error, refresh, upload, remove, reindex, accept } = useKnowledgeDocuments()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<KnowledgeDocumentStatus | 'all'>('all')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<KnowledgeDocument | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [isReindexing, setIsReindexing] = useState(false)

  const filteredDocuments = useMemo(() => filterKnowledgeDocuments(documents, query, status), [documents, query, status])
  const selected = documents.find((document) => document.file_id === selectedID) || filteredDocuments[0] || null

  const handleFiles = (files?: FileList | null) => {
    if (files?.length) void upload(files)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setIsDeleting(true)
    try {
      await remove(pendingDelete.file_id)
      if (selectedID === pendingDelete.file_id) setSelectedID(null)
      setPendingDelete(null)
    } catch {
      return
    } finally {
      setIsDeleting(false)
    }
  }

  const handleReindex = async (document: KnowledgeDocument) => {
    setIsReindexing(true)
    try {
      await reindex(document.file_id)
    } catch {
      return
    } finally {
      setIsReindexing(false)
    }
  }

  return (
    <section className="h-full overflow-y-auto px-4 py-6 sm:px-7 lg:px-9">
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
          <div>
            <p className="text-xs font-semibold tracking-[0.16em] text-sky-600">KNOWLEDGE LIBRARY</p>
            <h1 className="mt-1 text-2xl font-semibold tracking-tight text-zinc-900 dark:text-white">知识库</h1>
            <p className="mt-1.5 max-w-xl text-sm leading-6 text-zinc-500 dark:text-zinc-400">维护你自己的运维资料。只有状态为“已就绪”的资料会参与后续检索。</p>
          </div>
          <div className="flex items-center gap-2">
            <button type="button" onClick={() => void refresh()} className="rounded-xl border border-zinc-200 bg-white px-3 py-2 text-xs font-medium text-zinc-600 transition hover:border-sky-200 hover:text-sky-700 dark:border-slate-700 dark:bg-slate-900 dark:text-zinc-300" title="刷新资料列表"><RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} /></button>
            <input ref={fileInputRef} type="file" className="hidden" accept={accept} multiple onChange={(event) => { handleFiles(event.target.files); event.target.value = '' }} />
            <button type="button" onClick={() => fileInputRef.current?.click()} disabled={isUploading} className="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm shadow-sky-500/25 transition hover:bg-sky-500 disabled:cursor-not-allowed disabled:opacity-60">
              {isUploading ? <Loader2 size={16} className="animate-spin" /> : <Upload size={16} />}
              {isUploading ? '上传中…' : '上传资料'}
            </button>
          </div>
        </header>

        {error && <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-rose-200 bg-rose-50 px-3.5 py-3 text-sm text-rose-700 dark:border-rose-500/20 dark:bg-rose-500/10 dark:text-rose-300"><span>{error}</span><button onClick={() => void refresh()} className="text-xs font-semibold underline">重试</button></div>}

        <div className="flex flex-col gap-3 border-y border-zinc-200/80 py-3.5 dark:border-slate-700/70 sm:flex-row sm:items-center sm:justify-between">
          <label className="flex max-w-md flex-1 items-center gap-2 rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-500 focus-within:border-sky-300 focus-within:ring-2 focus-within:ring-sky-100 dark:border-slate-700 dark:bg-slate-900 dark:text-zinc-400 dark:focus-within:border-sky-500/60 dark:focus-within:ring-sky-500/10">
            <Search size={15} />
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="按文件名搜索" className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-zinc-400" />
          </label>
          <div className="flex gap-1 overflow-x-auto pb-0.5">
            {statusOptions.map((option) => <button key={option.value} type="button" onClick={() => setStatus(option.value)} className={`whitespace-nowrap rounded-lg px-2.5 py-1.5 text-xs font-medium transition ${status === option.value ? 'bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300' : 'text-zinc-500 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-slate-800'}`}>{option.label}</button>)}
          </div>
        </div>

        <div className="grid min-h-[420px] gap-5 lg:grid-cols-[minmax(0,1fr)_260px]">
          <div className="overflow-hidden rounded-2xl border border-white/70 bg-white/75 shadow-sm shadow-zinc-900/5 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/70">
            <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 border-b border-zinc-100 px-4 py-3 text-[11px] font-medium tracking-wide text-zinc-400 dark:border-slate-800 dark:text-zinc-500 sm:grid-cols-[minmax(0,1fr)_90px_110px_100px]">
              <span>资料</span><span className="hidden sm:block">状态</span><span className="hidden sm:block">上传时间</span><span>大小</span>
            </div>
            {isLoading && documents.length === 0 ? (
              <div className="flex h-72 items-center justify-center text-sm text-zinc-400"><Loader2 size={18} className="mr-2 animate-spin" />正在加载资料…</div>
            ) : filteredDocuments.length > 0 ? (
              <ul>
                {filteredDocuments.map((document) => {
                  const meta = knowledgeStatusMeta(document.status)
                  const active = selected?.file_id === document.file_id
                  return <li key={document.file_id}><button type="button" onClick={() => setSelectedID(document.file_id)} className={`grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-zinc-100 px-4 py-3 text-left transition last:border-b-0 sm:grid-cols-[minmax(0,1fr)_90px_110px_100px] dark:border-slate-800 ${active ? 'bg-sky-50/70 dark:bg-sky-500/10' : 'hover:bg-zinc-50/80 dark:hover:bg-slate-800/70'}`}>
                    <span className="flex min-w-0 items-center gap-3"><span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-300"><FileText size={17} /></span><span className="min-w-0"><span className="block truncate text-sm font-medium text-zinc-800 dark:text-zinc-100">{document.file_name}</span><span className="mt-0.5 block text-[11px] text-zinc-400 dark:text-zinc-500 sm:hidden">{meta.label} · {formatKnowledgeDate(document.uploaded_at)}</span></span></span>
                    <span className={`hidden w-fit items-center gap-1 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 sm:inline-flex ${meta.className}`}><StatusIcon status={document.status} />{meta.label}</span>
                    <span className="hidden text-xs text-zinc-400 sm:block dark:text-zinc-500">{formatKnowledgeDate(document.uploaded_at)}</span>
                    <span className="text-right text-xs text-zinc-400 dark:text-zinc-500">{formatKnowledgeFileSize(document.file_size)}</span>
                  </button></li>
                })}
              </ul>
            ) : (
              <div className="flex h-72 flex-col items-center justify-center px-6 text-center"><span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-sky-50 text-sky-500 dark:bg-sky-500/10"><FolderOpen size={22} /></span><p className="mt-4 text-sm font-semibold text-zinc-700 dark:text-zinc-200">{documents.length ? '没有匹配的资料' : '还没有知识资料'}</p><p className="mt-1 max-w-xs text-xs leading-5 text-zinc-500 dark:text-zinc-400">{documents.length ? '换个关键词或状态筛选试试。' : '上传 SOP、故障复盘或运行手册，让它成为后续问答的依据。'}</p>{!documents.length && <button type="button" onClick={() => fileInputRef.current?.click()} className="mt-4 text-xs font-semibold text-sky-600 hover:text-sky-500">上传第一份资料</button>}</div>
            )}
          </div>

          <aside className="rounded-2xl border border-white/70 bg-white/75 p-4 shadow-sm shadow-zinc-900/5 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/70">
            {selected ? (() => {
              const meta = knowledgeStatusMeta(selected.status)
              return <div><div className="flex items-start justify-between gap-3"><span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-300"><FileText size={19} /></span><span className={`inline-flex items-center gap-1 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ${meta.className}`}><StatusIcon status={selected.status} />{meta.label}</span></div><h2 className="mt-4 break-all text-sm font-semibold leading-5 text-zinc-800 dark:text-zinc-100">{selected.file_name}</h2><dl className="mt-5 space-y-3 text-xs"><div className="flex justify-between gap-3"><dt className="text-zinc-400">文件大小</dt><dd className="text-zinc-600 dark:text-zinc-300">{formatKnowledgeFileSize(selected.file_size)}</dd></div><div className="flex justify-between gap-3"><dt className="text-zinc-400">上传时间</dt><dd className="text-right text-zinc-600 dark:text-zinc-300">{formatKnowledgeDate(selected.uploaded_at)}</dd></div><div className="flex justify-between gap-3"><dt className="text-zinc-400">版本</dt><dd className="text-zinc-600 dark:text-zinc-300">v{selected.version || 1}</dd></div></dl><div className="mt-6 space-y-2">{selected.status === 'failed' && <button type="button" onClick={() => void handleReindex(selected)} disabled={isReindexing} className="flex w-full items-center justify-center gap-2 rounded-xl border border-sky-200 bg-sky-50 px-3 py-2 text-xs font-semibold text-sky-700 transition hover:bg-sky-100 disabled:opacity-60 dark:border-sky-500/20 dark:bg-sky-500/10 dark:text-sky-300"><RefreshCw size={14} className={isReindexing ? 'animate-spin' : ''} />重新索引</button>}{selected.status === 'indexing' ? <p className="px-1 text-center text-[11px] leading-5 text-zinc-400">资料索引完成后可删除。</p> : <button type="button" onClick={() => setPendingDelete(selected)} className="flex w-full items-center justify-center gap-2 rounded-xl px-3 py-2 text-xs font-semibold text-rose-600 transition hover:bg-rose-50 dark:text-rose-300 dark:hover:bg-rose-500/10"><Trash2 size={14} />删除资料</button>}</div></div>
            })() : <div className="flex h-full min-h-48 flex-col items-center justify-center text-center"><FileText size={20} className="text-zinc-300 dark:text-zinc-600" /><p className="mt-3 text-xs text-zinc-400">选择一份资料查看详情</p></div>}
          </aside>
        </div>
      </div>

      {pendingDelete && <div className="fixed inset-0 z-[70] flex items-end justify-center bg-slate-950/20 p-4 backdrop-blur-[2px] sm:items-center"><div role="dialog" aria-modal="true" aria-labelledby="delete-knowledge-title" className="w-full max-w-sm rounded-2xl border border-white/70 bg-white p-5 shadow-2xl shadow-slate-950/15 dark:border-slate-700 dark:bg-slate-900"><div className="flex items-start justify-between"><div><p className="text-xs font-semibold tracking-wide text-rose-600">删除资料</p><h2 id="delete-knowledge-title" className="mt-1 text-base font-semibold text-zinc-900 dark:text-white">确认移除这份资料？</h2></div><button onClick={() => setPendingDelete(null)} className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-slate-800"><X size={16} /></button></div><p className="mt-3 text-sm leading-6 text-zinc-500 dark:text-zinc-400">“{pendingDelete.file_name}”的原文件、元数据和检索内容都会被删除，之后将不能恢复。</p><div className="mt-5 grid grid-cols-2 gap-3"><button onClick={() => setPendingDelete(null)} disabled={isDeleting} className="rounded-xl border border-zinc-200 px-3 py-2.5 text-sm font-medium text-zinc-600 dark:border-slate-700 dark:text-zinc-300">取消</button><button onClick={() => void confirmDelete()} disabled={isDeleting} className="inline-flex items-center justify-center gap-2 rounded-xl bg-rose-600 px-3 py-2.5 text-sm font-medium text-white hover:bg-rose-500 disabled:opacity-60">{isDeleting && <Loader2 size={15} className="animate-spin" />}确认删除</button></div></div></div>}
    </section>
  )
}
