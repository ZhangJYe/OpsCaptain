import { useState } from 'react'
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

const TONE_CLASSES: Record<string, string> = {
  blue: 'bg-blue-500/10 text-blue-400 ring-blue-500/20',
  green: 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20',
  amber: 'bg-amber-500/10 text-amber-400 ring-amber-500/20',
  slate: 'bg-slate-500/10 text-slate-400 ring-slate-500/20',
}

export function OperatorCard() {
  const [operator] = useState(() => ROSTER[Math.floor(Math.random() * ROSTER.length)])

  return (
    <div className="rounded-xl border border-zinc-200/80 bg-white/80 p-4 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <div className="flex items-center gap-3">
        <div className={`flex h-10 w-10 items-center justify-center rounded-full text-lg font-bold ring-1 ring-inset ${TONE_CLASSES[operator.tone]}`}>
          {operator.name[0]}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-zinc-900 dark:text-white">{operator.name}</span>
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
          </div>
          <p className="text-xs text-zinc-500 dark:text-zinc-500">当前值班 · 证据整理中</p>
        </div>
        <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-400 ring-1 ring-inset ring-emerald-500/20">
          Live
        </span>
      </div>
    </div>
  )
}
