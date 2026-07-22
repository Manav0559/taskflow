import { Moon, Sun, SunMoon } from 'lucide-react'
import clsx from 'clsx'
import { useTheme } from '@/lib/theme'

const OPTIONS = [
  { value: 'light', icon: Sun, label: 'Light' },
  { value: 'system', icon: SunMoon, label: 'System' },
  { value: 'dark', icon: Moon, label: 'Dark' },
] as const

export function ThemeToggle() {
  const { preference, setPreference } = useTheme()

  return (
    <div className="flex items-center gap-0.5 rounded-lg border border-border bg-surface p-0.5">
      {OPTIONS.map(({ value, icon: Icon, label }) => (
        <button
          key={value}
          title={label}
          onClick={() => setPreference(value)}
          className={clsx(
            'flex h-6 w-6 items-center justify-center rounded-md transition-colors',
            preference === value
              ? 'bg-card text-ink shadow-sm'
              : 'text-ink-muted hover:text-ink-secondary',
          )}
        >
          <Icon size={13} />
        </button>
      ))}
    </div>
  )
}
