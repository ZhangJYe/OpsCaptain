import { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import type { ThinkingStep } from '../agent/ThinkingCollapse'

type PetMood = 'idle' | 'thinking' | 'done' | 'error' | 'gos'

interface MoodConfig {
  emoji: string
  label: string
  animation: string
  quips: string[]
}

const MOOD_CONFIG: Record<PetMood, MoodConfig> = {
  idle: {
    emoji: '🤖',
    label: '待命',
    animation: '',
    quips: ['有什么需要排查的？', '运维小助手随时待命', '丢个问题过来吧'],
  },
  thinking: {
    emoji: '🔍',
    label: '排查中',
    animation: 'animate-bounce',
    quips: ['正在翻日志...', '指标拉取中...', '让我看看发生了什么...'],
  },
  done: {
    emoji: '✅',
    label: '完成',
    animation: '',
    quips: ['搞定！', '排查完毕，请过目', '这次运气不错'],
  },
  error: {
    emoji: '⚠️',
    label: '异常',
    animation: 'animate-pulse',
    quips: ['这条路走不通，换个方向', '有异常，但别慌', '出了点状况'],
  },
  gos: {
    emoji: '🧠',
    label: '推理中',
    animation: 'animate-pulse',
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
  const isWorking = mood === 'thinking' || mood === 'gos'

  const handlePoke = () => {
    setQuip(pickQuip(mood))
    setShowBubble(true)
    clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setShowBubble(false), 3000)
  }

  return (
    <div className="flex items-center gap-3">
      {/* Pet character - inline with input */}
      <div className="relative flex flex-col items-center gap-1">
        <AnimatePresence>
          {showBubble && quip && (
            <motion.div
              initial={{ opacity: 0, y: 6, scale: 0.9 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 4, scale: 0.95 }}
              transition={{ type: 'spring', damping: 20, stiffness: 300 }}
              className="absolute bottom-full mb-2 w-max max-w-[160px] rounded-lg border border-zinc-200/80 bg-white px-2.5 py-1.5 text-[11px] text-zinc-600 shadow-md dark:border-zinc-700/60 dark:bg-zinc-800 dark:text-zinc-300"
            >
              {quip}
              <div className="absolute -bottom-1 left-4 h-2 w-2 rotate-45 border-b border-r border-zinc-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-800" />
            </motion.div>
          )}
        </AnimatePresence>

        <button
          type="button"
          aria-label={`运维助手 - ${config.label}，点击刷新`}
          className="relative flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-xl border border-zinc-200/80 bg-white transition-all hover:scale-105 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-accent dark:border-zinc-700/60 dark:bg-zinc-800"
          onClick={handlePoke}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              handlePoke()
            }
          }}
        >
          <span className={`text-lg ${config.animation}`}>{config.emoji}</span>
          {isWorking && (
            <span className="absolute -top-0.5 -right-0.5 flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent/60 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-accent" />
            </span>
          )}
        </button>

        <span className="text-[9px] text-zinc-400 dark:text-zinc-600">{config.label}</span>
      </div>
    </div>
  )
}
