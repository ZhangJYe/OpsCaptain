import { useMemo, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { BarChart3, FileSearch, BookOpen } from 'lucide-react'
import type { SkillDomain } from '../../types/chat'
import { SKILL_GROUPS, cn } from '../../lib/utils'

interface Props {
  selectedSkillIds: string[]
  onChange: (ids: string[]) => void
}

const domainMeta: Record<SkillDomain, { icon: typeof BarChart3; label: string; color: string; bg: string; border: string; dot: string }> = {
  metrics:    { icon: BarChart3, label: 'Metrics', color: 'text-blue-500',  bg: 'bg-blue-500/6',  border: 'border-blue-500/25',  dot: 'bg-blue-500' },
  logs:       { icon: FileSearch, label: 'Logs',    color: 'text-amber-500', bg: 'bg-amber-500/6',  border: 'border-amber-500/25', dot: 'bg-amber-500' },
  knowledge:  { icon: BookOpen,   label: 'Knowledge',color: 'text-emerald-500',bg: 'bg-emerald-500/6',border: 'border-emerald-500/25',dot: 'bg-emerald-500' },
}

export function SkillPanel({ selectedSkillIds, onChange }: Props) {
  const [activeDomain, setActiveDomain] = useState<SkillDomain>('metrics')

  const activeGroup = useMemo(
    () => SKILL_GROUPS.find((g) => g.id === activeDomain) ?? SKILL_GROUPS[0],
    [activeDomain]
  )

  const selectedSet = useMemo(() => new Set(selectedSkillIds), [selectedSkillIds])
  const activeCount = activeGroup.skills.filter((s) => selectedSet.has(s.id)).length
  const totalSelected = selectedSkillIds.length
  const dm = domainMeta[activeDomain]

  const toggleSkill = (skillId: string) => {
    const next = new Set(selectedSkillIds)
    next.has(skillId) ? next.delete(skillId) : next.add(skillId)
    onChange(Array.from(next))
  }

  return (
    <div className="rounded-xl border border-zinc-200/80 bg-white/80 p-3 backdrop-blur dark:border-zinc-800/60 dark:bg-zinc-900/60">
      {/* Header */}
      <div className="mb-3 flex items-center justify-between">
        <div>
          <p className="text-[11px] font-medium text-zinc-500 dark:text-zinc-500">Skills</p>
          <p className="mt-0.5 text-[10px] text-zinc-400 dark:text-zinc-600 truncate max-w-[160px]">
            {activeGroup.description}
          </p>
        </div>
        {totalSelected > 0 && (
          <span className="inline-flex items-center gap-1 rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium text-accent ring-1 ring-inset ring-accent/20">
            {totalSelected}
          </span>
        )}
      </div>

      {/* Domain tabs */}
      <div className="mb-3 flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
        {SKILL_GROUPS.map((group) => {
          const meta = domainMeta[group.id]
          const Icon = meta.icon
          const isActive = activeDomain === group.id
          return (
            <button
              key={group.id}
              onClick={() => setActiveDomain(group.id)}
              className={cn(
                'relative flex flex-1 items-center justify-center gap-1.5 rounded-md py-2 text-[11px] font-medium transition-all duration-200',
                isActive
                  ? 'text-zinc-900 dark:text-white'
                  : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200'
              )}
            >
              {isActive && (
                <motion.div
                  layoutId="skill-tab"
                  className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-zinc-200/60 dark:bg-zinc-700 dark:ring-zinc-600/60"
                  transition={{ type: 'spring', damping: 20, stiffness: 300 }}
                />
              )}
              <Icon size={12} className={cn('relative z-10', isActive && meta.color)} />
              <span className="relative z-10">{meta.label}</span>
            </button>
          )
        })}
      </div>

      {/* Active domain indicator */}
      <div className={cn('mb-3 flex items-center gap-2 rounded-lg px-3 py-2 text-xs', dm.bg, dm.border, 'border')}>
        <span className={cn('h-1.5 w-1.5 rounded-full', dm.dot)} />
        <span className="font-medium text-zinc-700 dark:text-zinc-300">{activeCount}/{activeGroup.skills.length} 已启用</span>
      </div>

      {/* Skill cards */}
      <div className="space-y-1.5">
        <AnimatePresence mode="wait">
          <motion.div
            key={activeDomain}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.2 }}
            className="space-y-1.5"
          >
            {activeGroup.skills.map((skill) => {
              const enabled = selectedSet.has(skill.id)
              return (
                <button
                  key={skill.id}
                  onClick={() => toggleSkill(skill.id)}
                  className={cn(
                    'group relative w-full rounded-xl border px-3 py-2.5 text-left transition-all duration-200',
                    enabled
                      ? cn('border-accent/30 bg-accent/5 hover:bg-accent/10', dm.color)
                      : 'border-zinc-200/80 bg-white/70 hover:border-zinc-300 hover:bg-white dark:border-zinc-800/60 dark:bg-zinc-950/50 dark:hover:border-zinc-700 dark:hover:bg-zinc-900/60'
                  )}
                >
                  <div className="flex items-start gap-3">
                    {/* Toggle switch */}
                    <div
                      className={cn(
                        'relative mt-0.5 h-4 w-8 shrink-0 rounded-full transition-colors duration-200',
                        enabled ? 'bg-accent' : 'bg-zinc-200 dark:bg-zinc-700'
                      )}
                    >
                      <motion.div
                        className="absolute top-0.5 h-3 w-3 rounded-full bg-white shadow-sm"
                        animate={{ left: enabled ? 18 : 2 }}
                        transition={{ type: 'spring', stiffness: 500, damping: 30 }}
                      />
                    </div>

                    <div className="min-w-0 flex-1">
                      <div className="text-[13px] font-medium text-zinc-800 dark:text-zinc-200">{skill.label}</div>
                      <div className="mt-0.5 text-[11px] leading-4 text-zinc-500 dark:text-zinc-500 line-clamp-2">
                        {skill.description}
                      </div>
                    </div>
                  </div>

                  {/* Selection glow */}
                  {enabled && (
                    <div className="pointer-events-none absolute inset-0 rounded-xl ring-1 ring-inset ring-accent/20" />
                  )}
                </button>
              )
            })}
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  )
}
