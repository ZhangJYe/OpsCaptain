import { useEffect, useRef, useState } from 'react'
import { motion } from 'framer-motion'
import { PetCharacter } from './PetCharacter'
import type { PetMood } from './PetCharacter'

interface Position {
  x: number
  y: number
}

interface Props {
  mood: PetMood
}

const STORAGE_KEY = 'opscaptain-floating-companion-position'
const SIZE = 64
const GAP = 20
const BOTTOM_OFFSET = 92

function clampPosition(position: Position): Position {
  return {
    x: Math.min(Math.max(GAP, position.x), Math.max(GAP, window.innerWidth - SIZE - GAP)),
    y: Math.min(Math.max(GAP, position.y), Math.max(GAP, window.innerHeight - SIZE - BOTTOM_OFFSET)),
  }
}

function defaultPosition(): Position {
  return {
    x: window.innerWidth - SIZE - GAP,
    y: window.innerHeight - SIZE - BOTTOM_OFFSET,
  }
}

function readPosition(): Position {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    const parsed = raw ? JSON.parse(raw) : null
    if (typeof parsed?.x === 'number' && typeof parsed?.y === 'number') {
      return clampPosition(parsed)
    }
  } catch {
    return defaultPosition()
  }
  return defaultPosition()
}

export function FloatingCompanion({ mood }: Props) {
  const [position, setPosition] = useState<Position | null>(null)
  const dragOffsetRef = useRef<Position | null>(null)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    setPosition(readPosition())

    const handleResize = () => {
      setPosition((current) => current && clampPosition(current))
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  useEffect(() => {
    if (!position) return
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(position))
    } catch {
      return
    }
  }, [position])

  if (!position) return null

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragOffsetRef.current = {
      x: event.clientX - position.x,
      y: event.clientY - position.y,
    }
    setDragging(true)
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const offset = dragOffsetRef.current
    if (!offset) return
    setPosition(clampPosition({
      x: event.clientX - offset.x,
      y: event.clientY - offset.y,
    }))
  }

  const handlePointerEnd = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!dragOffsetRef.current) return
    dragOffsetRef.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
    setDragging(false)
  }

  const active = mood === 'thinking' || mood === 'gos'

  return (
    <div
      className={`fixed z-40 touch-none select-none ${dragging ? 'cursor-grabbing' : 'cursor-grab'}`}
      style={{ left: position.x, top: position.y }}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerEnd}
      onPointerCancel={handlePointerEnd}
      aria-label={`运维助手${active ? '正在工作' : '待命'}`}
    >
      <motion.span
        aria-hidden="true"
        className="pointer-events-none absolute -inset-2 rounded-[24px] border border-sky-300/50 dark:border-sky-400/30"
        animate={active ? { opacity: [0.25, 0.8, 0.25], scale: [0.9, 1.08, 0.9] } : { opacity: 0.2, scale: 0.96 }}
        transition={active ? { duration: 1.8, repeat: Infinity, ease: 'easeInOut' } : { duration: 0.2 }}
      />
      <span className={`pointer-events-none absolute -right-0.5 top-0.5 z-10 h-3 w-3 rounded-full border-2 border-white dark:border-slate-900 ${active ? 'bg-sky-400 shadow-[0_0_10px_rgba(56,189,248,0.8)]' : 'bg-emerald-400'}`} />
      <PetCharacter mood={mood} size={SIZE} className="pointer-events-none" />
    </div>
  )
}
