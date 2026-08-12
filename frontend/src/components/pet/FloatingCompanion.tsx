import { useEffect, useRef, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { ChangeEventPanel } from '../change-events/ChangeEventPanel'
import { useChangeEvents } from '../../hooks/useChangeEvents'
import { changeEventTypeLabel, isPetClick, petSentinelVisualState } from '../../lib/changeEventPresentation'
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

function clampPosition(position: Position): Position {
  return {
    x: Math.min(Math.max(GAP, position.x), Math.max(GAP, window.innerWidth - SIZE - GAP)),
    y: Math.min(Math.max(GAP, position.y), Math.max(GAP, window.innerHeight - SIZE - GAP)),
  }
}

function defaultPosition(): Position {
  return {
    x: window.innerWidth - SIZE - GAP,
    y: window.innerHeight - SIZE - GAP,
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
  const dragStartRef = useRef<Position | null>(null)
  const [dragging, setDragging] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const [showNotice, setShowNotice] = useState(false)
  const { events, latestEvent, unreadCount, status, markRead, clear } = useChangeEvents()

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

  useEffect(() => {
    if (!latestEvent || unreadCount === 0 || panelOpen) {
      setShowNotice(false)
      return undefined
    }
    setShowNotice(true)
    const timer = window.setTimeout(() => setShowNotice(false), 5000)
    return () => window.clearTimeout(timer)
  }, [latestEvent?.event_id, panelOpen, unreadCount])

  if (!position) return null

  const handlePointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragOffsetRef.current = {
      x: event.clientX - position.x,
      y: event.clientY - position.y,
    }
    dragStartRef.current = { x: event.clientX, y: event.clientY }
    setDragging(true)
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
    const offset = dragOffsetRef.current
    if (!offset) return
    setPosition(clampPosition({
      x: event.clientX - offset.x,
      y: event.clientY - offset.y,
    }))
  }

  const handlePointerEnd = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!dragOffsetRef.current) return
    dragOffsetRef.current = null
    const dragStart = dragStartRef.current
    dragStartRef.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    setDragging(false)
    if (event.type === 'pointerup' && dragStart && isPetClick(dragStart, { x: event.clientX, y: event.clientY })) {
      setPanelOpen((open) => {
        const next = !open
        if (next) markRead()
        return next
      })
    }
  }

  const active = mood === 'thinking' || mood === 'gos'
  const sentinel = petSentinelVisualState(status, latestEvent, unreadCount)
  const alignPanelLeft = position.x < 188

  return (
    <div
      className="fixed z-40"
      style={{ left: position.x, top: position.y }}
    >
      <AnimatePresence>
        {showNotice && latestEvent && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.96 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 6, scale: 0.98 }}
            className={`pointer-events-none absolute bottom-[calc(100%+14px)] ${alignPanelLeft ? 'left-0' : 'right-0'} w-[min(270px,calc(100vw-24px))] rounded-2xl rounded-br-md border border-white/70 bg-white/95 px-3 py-2 shadow-xl shadow-zinc-900/10 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/95`}
          >
            <p className="truncate text-[12px] font-semibold text-zinc-800 dark:text-zinc-100">{latestEvent.service} {changeEventTypeLabel(latestEvent.event_type)}</p>
            <p className="mt-0.5 line-clamp-1 text-[11px] text-zinc-500 dark:text-zinc-400">{latestEvent.summary}</p>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {panelOpen && (
          <motion.div
            initial={{ opacity: 0, y: 14, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 10, scale: 0.98 }}
            transition={{ type: 'spring', damping: 24, stiffness: 260 }}
            className={`absolute bottom-[calc(100%+14px)] ${alignPanelLeft ? 'left-0' : 'right-0'}`}
            onPointerDown={(event) => event.stopPropagation()}
          >
            <ChangeEventPanel events={events} status={status} onClear={clear} onClose={() => setPanelOpen(false)} />
          </motion.div>
        )}
      </AnimatePresence>

      <button
        type="button"
        className={`relative touch-none select-none ${dragging ? 'cursor-grabbing' : 'cursor-grab'} focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-slate-900`}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerEnd}
        onPointerCancel={handlePointerEnd}
        aria-label={`变更哨兵：${sentinel.statusLabel}${active ? '，运维助手正在工作' : ''}`}
        aria-expanded={panelOpen}
        title="查看变更事件"
      >
      <motion.span
        aria-hidden="true"
        className={`pointer-events-none absolute -inset-2 rounded-[24px] border ${sentinel.ringClass}`}
        animate={active ? { opacity: [0.25, 0.8, 0.25], scale: [0.9, 1.08, 0.9] } : { opacity: 0.2, scale: 0.96 }}
        transition={active ? { duration: 1.8, repeat: Infinity, ease: 'easeInOut' } : { duration: 0.2 }}
      />
      <span className={`pointer-events-none absolute -right-0.5 top-0.5 z-10 h-3 w-3 rounded-full border-2 border-white dark:border-slate-900 ${sentinel.statusClass}`} />
      {unreadCount > 0 && (
        <span className="pointer-events-none absolute -right-2 -top-2 z-20 min-w-5 rounded-full bg-rose-500 px-1 text-center text-[10px] font-bold leading-5 text-white shadow-lg shadow-rose-500/30">
          {unreadCount > 9 ? '9+' : unreadCount}
        </span>
      )}
      <PetCharacter mood={mood} size={SIZE} className="pointer-events-none" />
      </button>
    </div>
  )
}
