import clsx from 'clsx'
import { ThemeToggle } from '@/components/ThemeToggle'
import { useHealthz } from '@/lib/queries'

export function Topbar({ title }: { title: string }) {
  const { data, isError } = useHealthz()
  const ok = !isError && data?.status === 'ok'

  return (
    <header className="flex h-16 shrink-0 items-center justify-between border-b border-border px-6">
      <h1 className="text-lg font-semibold text-ink">{title}</h1>
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-1.5 text-xs text-ink-muted">
          <span
            className={clsx('h-1.5 w-1.5 rounded-full', ok ? 'bg-good' : 'bg-critical')}
          />
          {ok ? 'API connected' : 'API unreachable'}
        </div>
        <ThemeToggle />
      </div>
    </header>
  )
}
