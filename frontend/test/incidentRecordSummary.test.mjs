import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/incidentRecordSummary.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 },
})
const { incidentRecordSummary, sortedIncidentRecords } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const baseIncident = {
  incident_id: 'inc-1',
  session_id: 'session-1',
  title: 'paymentservice 延迟升高',
  status: 'degraded',
  engine_strategy: 'plan_execute_replan',
  latest_summary: '# 诊断报告\n\n> **报告状态**：证据受限，Redis 连接池等待需要人工复核。',
  turns: [],
  events: [],
  created_at: 1,
  updated_at: 10,
}

test('列表摘要跳过 Markdown 标题并保留结论首段', () => {
  assert.equal(incidentRecordSummary(baseIncident), '报告状态：证据受限，Redis 连接池等待需要人工复核。')
})

test('进行中的事故在没有结论时展示过程提示', () => {
  assert.equal(incidentRecordSummary({ ...baseIncident, status: 'running', latest_summary: '' }), '诊断进行中，正在汇集过程与证据。')
})

test('事故记录按最近更新时间倒序排列', () => {
  const records = sortedIncidentRecords([
    { ...baseIncident, incident_id: 'older', updated_at: 4 },
    { ...baseIncident, incident_id: 'newer', updated_at: 12 },
  ])
  assert.deepEqual(records.map((item) => item.incident_id), ['newer', 'older'])
})

test('空事故列表保持为空，供列表视图展示创建引导', () => {
  assert.deepEqual(sortedIncidentRecords([]), [])
})
