import { NavLink } from 'react-router-dom'
import clsx from 'clsx'
import { LayoutDashboard, ListChecks, Server, Settings, Workflow } from 'lucide-react'

const NAV = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/jobs', label: 'Jobs', icon: ListChecks },
  { to: '/workers', label: 'Workers', icon: Server },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export function Sidebar() {
  return (
    <aside className="flex h-full w-56 shrink-0 flex-col border-r border-border bg-card">
      <div className="flex items-center gap-2 px-5 py-5">
        <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-series-1 text-white">
          <Workflow size={16} />
        </div>
        <span className="text-sm font-semibold text-ink">TaskFlow</span>
      </div>
      <nav className="flex flex-1 flex-col gap-0.5 px-3">
        {NAV.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              clsx(
                'flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-series-1/10 text-series-1'
                  : 'text-ink-secondary hover:bg-surface hover:text-ink',
              )
            }
          >
            <Icon size={16} />
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="px-5 py-4 text-xs text-ink-muted">Distributed job scheduler</div>
    </aside>
  )
}
