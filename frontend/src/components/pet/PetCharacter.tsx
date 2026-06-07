import { useEffect, useMemo, useRef, useState } from 'react'
import { useGSAP } from '@gsap/react'
import gsap from 'gsap'

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
  const rootRef = useRef<HTMLDivElement>(null)
  const active = mood === 'thinking' || mood === 'gos'
  const danger = mood === 'error'
  const done = mood === 'done'

  usePetMotion(rootRef, mood)

  return (
    <div
      ref={rootRef}
      aria-hidden="true"
      className={`relative flex items-center justify-center overflow-hidden rounded-[28px] border border-white/65 bg-white/80 shadow-lg shadow-zinc-900/5 backdrop-blur-md dark:border-white/10 dark:bg-slate-800/75 ${className}`}
      style={{ width: size, height: size }}
    >
      <span className="absolute inset-2 rounded-full bg-gradient-to-br from-sky-100 via-white to-amber-50 dark:from-sky-900/40 dark:via-slate-800 dark:to-slate-700" />
      <span className={`absolute right-3 top-3 h-2.5 w-2.5 rounded-full ${danger ? 'bg-rose-400' : done ? 'bg-emerald-400' : active ? 'bg-sky-400' : 'bg-zinc-300'} shadow-[0_0_12px_currentColor]`} />
      <span className="relative text-sm font-bold tracking-normal text-sky-500 dark:text-sky-300">OC</span>
    </div>
  )
}

function usePetMotion(
  ref: React.RefObject<HTMLElement>,
  mood: PetMood,
) {
  useGSAP(() => {
    const node = ref.current
    if (!node) return

    gsap.set(node, { opacity: 1, scale: 1, x: 0, y: 0, rotate: 0 })

    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (prefersReducedMotion) return

    const timeline = gsap.timeline()
    timeline.fromTo(
      node,
      { opacity: 0, scale: 0.94, y: 4 },
      { opacity: 1, scale: 1, y: 0, duration: 0.24, ease: 'back.out(1.8)' },
    )

    if (mood === 'thinking' || mood === 'gos') {
      timeline.to(node, {
        y: mood === 'gos' ? -5 : -3,
        rotate: mood === 'gos' ? 1.5 : 0,
        duration: mood === 'gos' ? 0.82 : 1.35,
        repeat: -1,
        yoyo: true,
        ease: 'sine.inOut',
      })
      return
    }

    if (mood === 'idle') {
      timeline.to(node, {
        y: -2,
        duration: 1.9,
        repeat: -1,
        yoyo: true,
        ease: 'sine.inOut',
      })
      return
    }

    if (mood === 'done') {
      timeline.to(node, {
        scale: 1.06,
        duration: 0.16,
        yoyo: true,
        repeat: 1,
        ease: 'power2.out',
      })
      return
    }

    if (mood === 'error') {
      timeline.to(node, {
        x: 3,
        rotate: 2,
        duration: 0.07,
        repeat: 5,
        yoyo: true,
        ease: 'power1.inOut',
      })
    }
  }, { dependencies: [mood], scope: ref, revertOnUpdate: true })
}

export function PetCharacter({ mood, size = 64, className = '' }: Props) {
  const rootRef = useRef<HTMLImageElement>(null)
  const filename = PET_FILES[mood]
  const candidates = useMemo(() => {
    const paths = [petAsset(filename), `/ai/pet/opscaptain/${filename}`, `/pet/opscaptain/${filename}`]
    return Array.from(new Set(paths))
  }, [filename])
  const [candidateIndex, setCandidateIndex] = useState(0)
  const src = candidates[candidateIndex]

  usePetMotion(rootRef, mood)

  useEffect(() => {
    setCandidateIndex(0)
  }, [filename])

  if (!src) {
    return <FallbackCharacter mood={mood} size={size} className={className} />
  }

  return (
    <img
      key={src}
      ref={rootRef}
      src={src}
      width={size}
      height={size}
      alt={`运维助手-${PET_LABELS[mood]}`}
      className={`select-none rounded-[28px] bg-white/95 object-cover p-1 ring-1 ring-white/75 drop-shadow-[0_18px_22px_rgba(15,23,42,0.16)] dark:bg-white/90 dark:ring-white/20 ${className}`}
      draggable={false}
      onError={() => setCandidateIndex((index) => index + 1)}
      style={{ width: size, height: size }}
    />
  )
}
