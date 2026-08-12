import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/incidentSop.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 },
})
const { buildIncidentSop, incidentSopFilename } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const incident = {
  incident_id: 'inc-20260812',
  session_id: 'session-1',
  title: 'paymentservice P95 延迟升高',
  status: 'completed',
  engine_strategy: 'gos_engine',
  latest_summary: '',
  created_at: new Date('2026-08-12T08:00:00Z').getTime(),
  updated_at: new Date('2026-08-12T08:06:00Z').getTime(),
  turns: [{
    turn_id: 'turn-1',
    incident_id: 'inc-20260812',
    user_query: 'paymentservice P95 升高',
    status: 'completed',
    result: 'Redis 连接池等待导致延迟升高。',
    created_at: new Date('2026-08-12T08:00:00Z').getTime(),
    finished_at: new Date('2026-08-12T08:05:00Z').getTime(),
  }],
  events: [{
    event_id: 'event-1',
    incident_id: 'inc-20260812',
    type: 'task_completed',
    message: 'Redis 连接池等待持续升高',
    created_at: new Date('2026-08-12T08:04:00Z').getTime(),
  }],
}

test('生成的 SOP 保留当前事故的结论、证据、策略和时间记录', () => {
  const output = buildIncidentSop(incident, new Date('2026-08-12T08:10:00Z'))

  assert.match(output, /待人工复核的 SOP 草稿/)
  assert.match(output, /事故编号：inc-20260812/)
  assert.match(output, /实际诊断策略：GoS/)
  assert.match(output, /Redis 连接池等待导致延迟升高。/)
  assert.match(output, /Redis 连接池等待持续升高/)
  assert.match(output, /创建日期：/)
  assert.match(output, /最近更新日期：/)
  assert.match(output, /完成日期：/)
  assert.match(output, /导出日期：/)
})

test('SOP 文件名会安全化标题并附带导出日期', () => {
  assert.equal(
    incidentSopFilename(incident, new Date('2026-08-12T08:10:00Z')),
    'opscaptain-sop-paymentservice-P95-延迟升高-20260812.md',
  )
})
