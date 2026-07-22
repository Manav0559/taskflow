import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { palette, type ThemeMode } from '@/lib/palette'

type Preference = ThemeMode | 'system'
const PREF_KEY = 'taskflow.theme'

function resolveSystem(): ThemeMode {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

interface ThemeState {
  mode: ThemeMode
  colors: (typeof palette)[ThemeMode]
  preference: Preference
  setPreference: (p: Preference) => void
}

const ThemeContext = createContext<ThemeState | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<Preference>(
    () => (localStorage.getItem(PREF_KEY) as Preference | null) ?? 'system',
  )
  const [mode, setMode] = useState<ThemeMode>(() =>
    preference === 'system' ? resolveSystem() : preference,
  )

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', mode)
  }, [mode])

  useEffect(() => {
    if (preference !== 'system') {
      setMode(preference)
      return
    }
    setMode(resolveSystem())
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const listener = () => setMode(resolveSystem())
    mq.addEventListener('change', listener)
    return () => mq.removeEventListener('change', listener)
  }, [preference])

  const value = useMemo<ThemeState>(
    () => ({
      mode,
      colors: palette[mode],
      preference,
      setPreference: (p: Preference) => {
        localStorage.setItem(PREF_KEY, p)
        setPreferenceState(p)
      },
    }),
    [mode, preference],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeState {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}
