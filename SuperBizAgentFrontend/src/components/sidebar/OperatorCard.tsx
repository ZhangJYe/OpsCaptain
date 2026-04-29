import { useState, useEffect } from 'react'
import type { Operator } from '../../types/chat'

const ROSTER: Operator[] = [
  { name: '林澈', tone: 'blue' },
  { name: '许知安', tone: 'green' },
  { name: '周望', tone: 'amber' },
  { name: '陈序', tone: 'slate' },
  { name: '沈宁', tone: 'blue' },
  { name: '陆遥', tone: 'green' },
  { name: '顾川', tone: 'amber' },
  { name: '叶岚', tone: 'slate' },
]

const TONE_COLORS: Record<string, string> = {
  blue: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  green: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
  amber: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  slate: 'bg-slate-500/20 text-slate-400 border-slate-500/30',
}

export function OperatorCard() {
  const [operator] = useState(() => ROSTER[Math.floor(Math.random() * ROSTER.length)])

  return (
    <div className="glass rounded-xl p-4">
      <div className="flex items-center gap-3">
        <div className={`w-10 h-10 rounded-xl flex items-center justify-center font-bold text-lg border ${TONE_COLORS[operator.tone]}`}>
          {operator.name[0]}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{operator.name}</span>
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
            </span>
          </div>
          <p className="text-xs text-zinc-500">当前值班 · 整理证据，生成处理建议</p>
        </div>
        <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-medium">
          Live
        </span>
      </div>
    </div>
  )
}
