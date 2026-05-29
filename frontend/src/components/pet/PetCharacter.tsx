import { useEffect, useMemo, useState } from 'react'
import { motion } from 'framer-motion'

export type PetMood = 'idle' | 'thinking' | 'done' | 'error' | 'gos'

interface Props {
  mood: PetMood
  size?: number
  className?: string
}

function petAsset(filename: string): string {
  const rawBase = (import.meta as unknown as { env?: { BASE_URL?: string } }).env?.BASE_URL || './'
  const base = rawBase.endsWith('/') ? rawBase : `${rawBase}/`
  return `${base}pet/opscaptain/${filename}`
}

const PET_FILES: Record<PetMood, string> = {
  idle: 'idle.gif',
  thinking: 'thinking.gif',
  done: 'done.gif',
  error: 'error.gif',
  gos: 'gos.gif',
}

const PET_LABELS: Record<PetMood, string> = {
  idle: '待命',
  thinking: '排查中',
  done: '完成',
  error: '异常',
  gos: '推理中',
}

function FallbackCharacter({ mood, size, className }: Required<Props>) {
  const active = mood === 'thinking' || mood === 'gos'
  const danger = mood === 'error'
  const done = mood === 'done'

  return (
    <motion.div
      aria-hidden="true"
      className={`relative flex items-center justify-center overflow-hidden rounded-[28px] border border-white/65 bg-white/80 shadow-lg shadow-zinc-900/5 backdrop-blur-md dark:border-white/10 dark:bg-slate-800/75 ${className}`}
      style={{ width: size, height: size }}
      animate={active ? { y: [0, -3, 0] } : danger ? { rotate: [-2, 2, -2, 0] } : undefined}
      transition={active ? { duration: 1.4, repeat: Infinity, ease: 'easeInOut' } : danger ? { duration: 0.3, repeat: 2 } : undefined}
    >
      <span className="absolute inset-2 rounded-full bg-gradient-to-br from-sky-100 via-white to-amber-50 dark:from-sky-900/40 dark:via-slate-800 dark:to-slate-700" />
      <span className={`absolute right-3 top-3 h-2.5 w-2.5 rounded-full ${danger ? 'bg-rose-400' : done ? 'bg-emerald-400' : active ? 'bg-sky-400' : 'bg-zinc-300'} shadow-[0_0_12px_currentColor]`} />
      <span className="relative text-sm font-bold tracking-normal text-sky-500 dark:text-sky-300">OC</span>
    </motion.div>
  )
}

export function PetCharacter({ mood, size = 64, className = '' }: Props) {
  const filename = PET_FILES[mood]
  const candidates = useMemo(() => {
    const paths = [petAsset(filename), `/ai/pet/opscaptain/${filename}`, `/pet/opscaptain/${filename}`]
    return Array.from(new Set(paths))
  }, [filename])
  const [candidateIndex, setCandidateIndex] = useState(0)
  const active = mood === 'thinking' || mood === 'gos' || mood === 'idle'
  const src = candidates[candidateIndex]

  useEffect(() => {
    setCandidateIndex(0)
  }, [filename])

  if (!src) {
    return <FallbackCharacter mood={mood} size={size} className={className} />
  }

  return (
    <motion.img
      key={src}
      src={src}
      width={size}
      height={size}
      alt={`运维助手-${PET_LABELS[mood]}`}
      className={`select-none rounded-[28px] bg-white/95 object-cover p-1 ring-1 ring-white/75 drop-shadow-[0_18px_22px_rgba(15,23,42,0.16)] dark:bg-white/90 dark:ring-white/20 ${className}`}
      draggable={false}
      onError={() => setCandidateIndex((index) => index + 1)}
      initial={{ opacity: 0, scale: 0.94 }}
      animate={active ? { opacity: 1, scale: 1, y: [0, -3, 0] } : { opacity: 1, scale: 1, y: 0 }}
      transition={active ? { y: { duration: 2.8, repeat: Infinity, ease: 'easeInOut' }, opacity: { duration: 0.18 }, scale: { duration: 0.18 } } : { duration: 0.18 }}
      style={{ width: size, height: size }}
    />
  )
}
