import { useMemo, useState } from 'react'
import { Braces, Database, FileText } from 'lucide-react'
import type { SkillDomain } from '../../types/chat'
import { SKILL_GROUPS, cn } from '../../lib/utils'

interface Props {
  selectedSkillIds: string[]
  onChange: (ids: string[]) => void
}

const domainIcons: Record<SkillDomain, typeof Database> = {
  metrics: Database,
  logs: FileText,
  knowledge: Braces,
}

export function SkillPanel({ selectedSkillIds, onChange }: Props) {
  const [activeDomain, setActiveDomain] = useState<SkillDomain>('metrics')

  const activeGroup = useMemo(
    () => SKILL_GROUPS.find((group) => group.id === activeDomain) ?? SKILL_GROUPS[0],
    [activeDomain]
  )

  const activeCount = useMemo(() => {
    const selected = new Set(selectedSkillIds)
    return activeGroup.skills.filter((skill) => selected.has(skill.id)).length
  }, [activeGroup, selectedSkillIds])

  const toggleSkill = (skillId: string) => {
    const selected = new Set(selectedSkillIds)
    if (selected.has(skillId)) {
      selected.delete(skillId)
    } else {
      selected.add(skillId)
    }
    onChange(Array.from(selected))
  }

  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <p className="text-xs text-zinc-600 dark:text-zinc-500">Skills</p>
          <p className="mt-1 text-[11px] text-zinc-500 dark:text-zinc-600">{activeGroup.description}</p>
        </div>
        <span className="rounded-full border border-accent/20 bg-accent/10 px-2.5 py-1 text-[10px] font-medium text-accent">
          {activeCount} 已启用
        </span>
      </div>

      <div className="mb-3 inline-flex w-full rounded-2xl border border-zinc-200/80 bg-zinc-100/80 p-1 dark:border-zinc-800/80 dark:bg-zinc-950/70">
        {SKILL_GROUPS.map((group) => {
          const Icon = domainIcons[group.id]
          return (
            <button
              key={group.id}
              onClick={() => setActiveDomain(group.id)}
              className={cn(
                'flex flex-1 items-center justify-center gap-1.5 rounded-2xl px-2 py-2 text-[11px] font-medium transition-colors',
                activeDomain === group.id
                  ? 'bg-white text-zinc-900 shadow-sm dark:bg-accent/16 dark:text-accent'
                  : 'text-zinc-500 hover:text-zinc-800 dark:text-zinc-500 dark:hover:text-zinc-300'
              )}
            >
              <Icon size={13} />
              <span>{group.label}</span>
            </button>
          )
        })}
      </div>

      <div className="space-y-2">
        {activeGroup.skills.map((skill) => {
          const selected = selectedSkillIds.includes(skill.id)
          return (
            <button
              key={skill.id}
              onClick={() => toggleSkill(skill.id)}
              className={cn(
                'w-full rounded-2xl border px-3 py-3 text-left transition-all',
                selected
                  ? 'border-accent/30 bg-accent/10 text-zinc-900 dark:text-zinc-100'
                  : 'border-zinc-200/80 bg-white/70 text-zinc-900 hover:border-zinc-300 hover:bg-white dark:border-zinc-800/80 dark:bg-zinc-950/50 dark:text-zinc-100 dark:hover:border-zinc-700 dark:hover:bg-zinc-900/70'
              )}
              aria-pressed={selected}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-medium">{skill.label}</div>
                  <div className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-500">{skill.description}</div>
                </div>
                <span
                  className={cn(
                    'mt-0.5 inline-flex h-5 min-w-[2.5rem] items-center justify-center rounded-full px-2 text-[10px] font-medium',
                    selected
                      ? 'bg-accent text-white'
                      : 'bg-zinc-100 text-zinc-500 dark:bg-zinc-900 dark:text-zinc-500'
                  )}
                >
                  {selected ? '启用中' : '未启用'}
                </span>
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
