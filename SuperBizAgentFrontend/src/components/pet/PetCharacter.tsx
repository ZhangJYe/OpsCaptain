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
    label: '待命中',
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
    label: '已完成',
    animation: '',
    quips: ['搞定！', '排查完毕，请过目', '这次运气不错'],
  },
  error: {
    emoji: '⚠️',
    label: '遇到问题',
    animation: 'animate-pulse',
    quips: ['这条路走不通，换个方向', '有异常，但别慌', '出了点状况'],
  },
  gos: {
    emoji: '🧠',
    label: '信念推理',
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

export function PetCharacter({ steps, isStreaming, isGoS }: Props) {
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

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col items-end gap-2 select-none sm:bottom-8 sm:right-8">
      <AnimatePresence>
        {showBubble && quip && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.9 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 4, scale: 0.95 }}
            transition={{ type: 'spring', damping: 20, stiffness: 300 }}
            className="relative max-w-[200px] rounded-xl border border-zinc-200/80 bg-white px-3 py-2 text-xs text-zinc-600 shadow-lg dark:border-zinc-700/60 dark:bg-zinc-800 dark:text-zinc-300"
          >
            {quip}
            <div className="absolute -bottom-1.5 right-5 h-3 w-3 rotate-45 border-b border-r border-zinc-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-800" />
          </motion.div>
        )}
      </AnimatePresence>

      <motion.div
        whileHover={{ scale: 1.1 }}
        whileTap={{ scale: 0.95 }}
        className="relative flex h-14 w-14 cursor-pointer items-center justify-center rounded-2xl border border-zinc-200/80 bg-white shadow-lg transition-shadow hover:shadow-xl dark:border-zinc-700/60 dark:bg-zinc-800"
        onClick={() => {
          setQuip(pickQuip(mood))
          setShowBubble(true)
          clearTimeout(timerRef.current)
          timerRef.current = setTimeout(() => setShowBubble(false), 3000)
        }}
      >
        <span className={`text-2xl ${config.animation}`}>{config.emoji}</span>
        {isWorking && (
          <span className="absolute -top-1 -right-1 flex h-3 w-3">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent/60 opacity-75" />
            <span className="relative inline-flex h-3 w-3 rounded-full bg-accent" />
          </span>
        )}
      </motion.div>

      <span className="text-[10px] text-zinc-400 dark:text-zinc-600">
        {config.label}
      </span>
    </div>
  )
}
