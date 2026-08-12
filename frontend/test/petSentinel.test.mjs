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

test('人偶哨兵根据事件风险和连接状态映射状态', async () => {
  const { petSentinelVisualState } = await loadModule('../src/lib/changeEventPresentation.ts')
  const latestEvent = { risk_level: 'critical' }

  assert.match(petSentinelVisualState('open', latestEvent, 1).ringClass, /rose/)
  assert.match(petSentinelVisualState('error', latestEvent, 0).statusClass, /rose/)
  assert.equal(petSentinelVisualState('open', latestEvent, 0).statusLabel, '监听中')
})

test('拖动超过阈值时不应被识别为点击', async () => {
  const { isPetClick } = await loadModule('../src/lib/changeEventPresentation.ts')

  assert.equal(isPetClick({ x: 10, y: 10 }, { x: 13, y: 14 }), true)
  assert.equal(isPetClick({ x: 10, y: 10 }, { x: 17, y: 10 }), false)
})
