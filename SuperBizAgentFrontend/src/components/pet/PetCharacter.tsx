import { useEffect, useState } from 'react'
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

const PET_ASSETS: Record<PetMood, string> = {
  idle: petAsset('idle.gif'),
  thinking: petAsset('thinking.gif'),
  done: petAsset('done.gif'),
  error: petAsset('error.gif'),
  gos: petAsset('gos.gif'),
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
      className={`relative flex items-center justify-center rounded-2xl border border-white/60 bg-white/75 shadow-sm backdrop-blur-md dark:border-white/10 dark:bg-slate-800/70 ${className}`}
      style={{ width: size, height: size }}
      animate={active ? { y: [0, -3, 0] } : danger ? { rotate: [-2, 2, -2, 0] } : undefined}
      transition={active ? { duration: 1.4, repeat: Infinity, ease: 'easeInOut' } : danger ? { duration: 0.3, repeat: 2 } : undefined}
    >
      <span className={`h-3 w-3 rounded-full ${danger ? 'bg-rose-400' : done ? 'bg-emerald-400' : active ? 'bg-sky-400' : 'bg-zinc-300'} shadow-[0_0_12px_currentColor]`} />
      <span className="absolute bottom-1.5 text-[9px] font-semibold text-zinc-400 dark:text-zinc-500">OC</span>
    </motion.div>
  )
}

export function PetCharacter({ mood, size = 64, className = '' }: Props) {
  const src = PET_ASSETS[mood]
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  const active = mood === 'thinking' || mood === 'gos' || mood === 'idle'

  useEffect(() => {
    setFailedSrc(null)
  }, [src])

  if (failedSrc === src) {
    return <FallbackCharacter mood={mood} size={size} className={className} />
  }

  return (
    <motion.img
      key={src}
      src={src}
      width={size}
      height={size}
      alt={`运维助手-${PET_LABELS[mood]}`}
      className={`select-none object-contain drop-shadow-[0_18px_22px_rgba(15,23,42,0.18)] ${className}`}
      draggable={false}
      onError={() => setFailedSrc(src)}
      initial={{ opacity: 0, scale: 0.94 }}
      animate={active ? { opacity: 1, scale: 1, y: [0, -3, 0] } : { opacity: 1, scale: 1, y: 0 }}
      transition={active ? { y: { duration: 2.8, repeat: Infinity, ease: 'easeInOut' }, opacity: { duration: 0.18 }, scale: { duration: 0.18 } } : { duration: 0.18 }}
      style={{ width: size, height: size }}
    />
  )
}
