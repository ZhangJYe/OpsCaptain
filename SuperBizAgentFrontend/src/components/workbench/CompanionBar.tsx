import { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { PetCharacter } from '../pet/PetCharacter'
import type { PetMood } from '../pet/PetCharacter'
import type { ThinkingStep } from '../agent/ThinkingCollapse'

interface MoodConfig {
  label: string
  quips: string[]
}

const MOOD_CONFIG: Record<PetMood, MoodConfig> = {
  idle: {
    label: '待命',
    quips: ['有什么需要排查的？', '运维小助手随时待命', '丢个问题过来吧'],
  },
  thinking: {
    label: '排查中',
    quips: ['正在翻日志...', '指标拉取中...', '让我看看发生了什么...'],
  },
  done: {
    label: '完成',
    quips: ['搞定！', '排查完毕，请过目', '这次运气不错'],
  },
  error: {
    label: '异常',
    quips: ['这条路走不通，换个方向', '有异常，但别慌', '出了点状况'],
  },
  gos: {
    label: '推理中',
    quips: ['建立假设中...', '正在收敛推理链...', 'GoS 引擎全速运转'],
  },
}

function resolveMood(steps: ThinkingStep[], isStreaming: boolean, isGoS: boolean): PetMood {
  if (isGoS && isStreaming) return 'gos'
  if (steps.some((s) => s.status === 'error')) return 'error'
  if (isStreaming || steps.some((s) => s.status === 'active')) return 'thinking'
  if (steps.length > 0 && steps.every((s) => s.status === 'done')) return 'done'
  return 'idle'
}

function pickQuip(mood: PetMood): string {
  const quips = MOOD_CONFIG[mood].quips
  return quips[Math.floor(Math.random() * quips.length)]
}

interface Props {
  steps: ThinkingStep[]
  isStreaming: boolean
  isGoS: boolean
}

export function CompanionBar({ steps, isStreaming, isGoS }: Props) {
  const [mood, setMood] = useState<PetMood>('idle')
  const [quip, setQuip] = useState('')
  const [showBubble, setShowBubble] = useState(false)
  const prevMoodRef = useRef<PetMood>('idle')
  const timerRef = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    const newMood = resolveMood(steps, isStreaming, isGoS)
    if (newMood !== prevMoodRef.current) {
      prevMoodRef.current = newMood
      setMood(newMood)
      setQuip(pickQuip(newMood))
      setShowBubble(true)

      clearTimeout(timerRef.current)
      if (newMood === 'done' || newMood === 'error') {
        timerRef.current = setTimeout(() => setShowBubble(false), 4000)
      }
    }
  }, [steps, isStreaming, isGoS])

  useEffect(() => {
    return () => clearTimeout(timerRef.current)
  }, [])

  const config = MOOD_CONFIG[mood]

  const handlePoke = () => {
    setQuip(pickQuip(mood))
    setShowBubble(true)
    clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setShowBubble(false), 3000)
  }

  return (
    <div className="relative flex h-[86px] w-[78px] shrink-0 items-end justify-center sm:h-[112px] sm:w-[112px]">
      <motion.div
        aria-hidden="true"
        className="absolute bottom-1 h-7 w-16 rounded-full bg-sky-300/25 blur-xl dark:bg-sky-400/15 sm:h-9 sm:w-24"
        animate={{ opacity: [0.45, 0.75, 0.45], scale: [0.92, 1.05, 0.92] }}
        transition={{ duration: 3.2, repeat: Infinity, ease: 'easeInOut' }}
      />

      <AnimatePresence>
        {showBubble && quip && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.92 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 5, scale: 0.96 }}
            transition={{ type: 'spring', damping: 22, stiffness: 320 }}
            className="pointer-events-none absolute bottom-[76px] left-1/2 z-20 w-max max-w-[210px] -translate-x-1/2 whitespace-normal break-words rounded-2xl rounded-br-md border border-white/70 bg-white/95 px-3.5 py-2 text-center text-[12px] font-medium leading-snug text-zinc-800 shadow-xl shadow-zinc-900/10 backdrop-blur-md dark:border-white/15 dark:bg-slate-800/95 dark:text-zinc-100 sm:bottom-[98px] sm:max-w-[240px]"
          >
            {quip}
            <div className="absolute -bottom-1.5 left-1/2 -translate-x-1/2 border-[6px] border-transparent border-t-white/95 dark:border-t-slate-800/95" />
          </motion.div>
        )}
      </AnimatePresence>

      <button
        type="button"
        aria-label={`运维助手 - ${config.label}，点击互动`}
        className="group relative z-10 -mb-1 cursor-pointer rounded-3xl p-0.5 transition-transform hover:scale-[1.04] focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/50 active:scale-95"
        onClick={handlePoke}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            handlePoke()
          }
        }}
      >
        <PetCharacter mood={mood} size={88} />
      </button>
    </div>
  )
}
