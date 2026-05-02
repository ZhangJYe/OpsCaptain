import { Bot, Gauge, ShieldCheck } from 'lucide-react'

export function OperatorCard() {
  return (
    <div className="rounded-xl border border-zinc-200/80 bg-white/80 p-4 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent/10 text-accent ring-1 ring-inset ring-accent/20">
          <Bot size={18} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-zinc-900 dark:text-zinc-100">OpsCaption</span>
            <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-500 ring-1 ring-inset ring-emerald-500/20">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              ready
            </span>
          </div>
          <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-500">
            更像一个安静的工作代理，少介绍自己，多把证据和结论摆清楚。
          </p>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2 text-[11px] text-zinc-500 dark:text-zinc-500">
        <div className="rounded-lg border border-zinc-200/80 bg-zinc-50/90 px-3 py-2 dark:border-zinc-800/60 dark:bg-zinc-950/50">
          <div className="flex items-center gap-1.5">
            <Gauge size={12} className="text-accent" />
            <span>mode</span>
          </div>
          <div className="mt-1 text-zinc-700 dark:text-zinc-300">direct / stream</div>
        </div>
        <div className="rounded-lg border border-zinc-200/80 bg-zinc-50/90 px-3 py-2 dark:border-zinc-800/60 dark:bg-zinc-950/50">
          <div className="flex items-center gap-1.5">
            <ShieldCheck size={12} className="text-accent" />
            <span>behavior</span>
          </div>
          <div className="mt-1 text-zinc-700 dark:text-zinc-300">evidence first</div>
        </div>
      </div>
    </div>
  )
}
