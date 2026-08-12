import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

async function loadModule(path) {
  const source = await readFile(new URL(path, import.meta.url), 'utf8')
  const output = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText
  return import(`data:text/javascript;base64,${Buffer.from(output).toString('base64')}`)
}

const documents = [
  { file_id: '1', file_name: '支付服务 SOP.md', status: 'ready', file_size: 2048, uploaded_at: '2026-08-12T08:00:00Z', mime_type: 'text/markdown', version: 1 },
  { file_id: '2', file_name: '回滚复盘.pdf', status: 'failed', file_size: 1024, uploaded_at: '2026-08-12T09:00:00Z', mime_type: 'application/pdf', version: 2 },
]

test('知识资料可按状态与文件名筛选', async () => {
  const { filterKnowledgeDocuments } = await loadModule('../src/lib/knowledgeDocuments.ts')

  assert.deepEqual(filterKnowledgeDocuments(documents, '支付', 'all').map((item) => item.file_id), ['1'])
  assert.deepEqual(filterKnowledgeDocuments(documents, '', 'failed').map((item) => item.file_id), ['2'])
})

test('知识资料状态提供可读的展示文案', async () => {
  const { knowledgeStatusMeta, formatKnowledgeFileSize } = await loadModule('../src/lib/knowledgeDocuments.ts')

  assert.equal(knowledgeStatusMeta('ready').label, '已就绪')
  assert.equal(knowledgeStatusMeta('failed').label, '索引失败')
  assert.equal(formatKnowledgeFileSize(2 * 1024 * 1024), '2.0 MB')
})

test('删除资料必须经过明确确认', async () => {
  const source = await readFile(new URL('../src/components/knowledge/KnowledgeBaseView.tsx', import.meta.url), 'utf8')

  assert.match(source, /确认移除这份资料/)
  assert.match(source, /确认删除/)
})
