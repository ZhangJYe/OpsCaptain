import type { AIOpsEngine } from '../types/chat'

export interface EngineViewModel {
  label: string
  badge: string
  description: string
  flow: string[]
  trace: string
  cta: string
  resultTitle: string
  resultSubtitle: string
  resultPrimaryAction: string
  action: string
  actionButton: string
  sidebar: {
    selected: string
    idle: string
    icon: string
    flowActive: string
    dot: string
  }
  draft: {
    title: string
    intro: string
    steps: Array<{ label: string; detail: string }>
    footer: string
  }
}

export const ENGINE_VIEW_MODEL: Record<AIOpsEngine, EngineViewModel> = {
  plan_execute_replan: {
    label: 'Plan',
    badge: 'Plan-Execute',
    description: '线性编排排障步骤，适合按 runbook 稳定推进。',
    flow: ['Plan', 'Execute', 'Replan'],
    trace: '计划 → 执行 → 重规划 → 报告',
    cta: '启动排障计划',
    resultTitle: '诊断结果',
    resultSubtitle: '按线性排障计划整理风险、证据和下一步动作。',
    resultPrimaryAction: '继续排查',
    action: 'text-sky-600 hover:bg-sky-50/80 hover:text-sky-700 dark:text-sky-400 dark:hover:bg-sky-500/10 dark:hover:text-sky-300',
    actionButton: 'hover:text-sky-600 dark:hover:text-sky-400',
    sidebar: {
      selected: 'border-sky-300/70 bg-sky-50/80 text-sky-950 shadow-sm shadow-sky-500/10 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-100',
      idle: 'border-white/60 bg-white/55 text-zinc-700 hover:border-sky-300/60 hover:bg-sky-50/60 dark:border-white/10 dark:bg-slate-800/40 dark:text-zinc-300 dark:hover:border-sky-500/30 dark:hover:bg-sky-500/10',
      icon: 'bg-sky-500/12 text-sky-600 ring-sky-400/20 dark:bg-sky-400/10 dark:text-sky-300',
      flowActive: 'bg-sky-500/10 text-sky-700 ring-sky-400/20 dark:bg-sky-400/10 dark:text-sky-300',
      dot: 'bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.45)]',
    },
    draft: {
      title: '线性 Runbook',
      intro: '按值班排障顺序推进，先控面再收证据，最后形成处置建议。',
      steps: [
        { label: '识别影响面', detail: '服务、时间窗、风险等级' },
        { label: '拉取证据', detail: 'metrics / logs / knowledge' },
        { label: '生成处置建议', detail: '回滚、限流、验证步骤' },
      ],
      footer: '适合明确告警、发布回看、容量压测等线性排查。',
    },
  },
  gos_engine: {
    label: 'GoS',
    badge: 'Belief Graph',
    description: '先建立候选根因，再用证据支持或反驳，适合不确定故障。',
    flow: ['Hypothesis', 'Evidence', 'Confidence'],
    trace: '假设 → 证据 → 置信度',
    cta: '启动信念推理',
    resultTitle: 'GoS 信念报告',
    resultSubtitle: '围绕主假设、支持证据和置信度收敛组织报告。',
    resultPrimaryAction: '补充证据',
    action: 'text-amber-600 hover:bg-amber-50/80 hover:text-amber-700 dark:text-amber-400 dark:hover:bg-amber-500/10 dark:hover:text-amber-300',
    actionButton: 'hover:text-amber-600 dark:hover:text-amber-400',
    sidebar: {
      selected: 'border-amber-300/70 bg-amber-50/80 text-amber-950 shadow-sm shadow-amber-500/10 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100',
      idle: 'border-white/60 bg-white/55 text-zinc-700 hover:border-amber-300/60 hover:bg-amber-50/60 dark:border-white/10 dark:bg-slate-800/40 dark:text-zinc-300 dark:hover:border-amber-500/30 dark:hover:bg-amber-500/10',
      icon: 'bg-amber-500/12 text-amber-600 ring-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300',
      flowActive: 'bg-amber-500/10 text-amber-700 ring-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300',
      dot: 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.45)]',
    },
    draft: {
      title: '信念面板',
      intro: '把异常先转成候选根因，再收集支持/反驳证据，最后校准置信度。',
      steps: [
        { label: '候选根因', detail: '从症状抽取故障方向' },
        { label: '支持证据', detail: '挂载日志、指标、知识片段' },
        { label: '置信度', detail: '收敛 frontier 与降级判断' },
      ],
      footer: '适合线索不足、多根因竞争、需要解释推理链的场景。',
    },
  },
}

export function getEngineViewModel(engine: AIOpsEngine): EngineViewModel {
  return ENGINE_VIEW_MODEL[engine]
}
