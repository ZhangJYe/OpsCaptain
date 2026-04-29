import { useState, useEffect, useCallback } from 'react'

type Theme = 'light' | 'dark'

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() => {
    if (typeof window === 'undefined') return 'dark'
    const stored = localStorage.getItem('opscaption-theme')
    if (stored === 'light' || stored === 'dark') return stored
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  })

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    localStorage.setItem('opscaption-theme', theme)
  }, [theme])

  const toggle = useCallback(() => {
    setThemeState((prev) => (prev === 'dark' ? 'light' : 'dark'))
  }, [])

  return { theme, toggle }
}
