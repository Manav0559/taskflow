import { Route, Routes, useLocation } from 'react-router-dom'
import { Sidebar } from '@/components/Sidebar'
import { Topbar } from '@/components/Topbar'
import { ConnectScreen } from '@/components/ConnectScreen'
import { useConnection } from '@/lib/connection'
import { Overview } from '@/pages/Overview'
import { Jobs } from '@/pages/Jobs'
import { JobDetail } from '@/pages/JobDetail'
import { RunDetail } from '@/pages/RunDetail'
import { Workers } from '@/pages/Workers'
import { Settings } from '@/pages/Settings'

const TITLES: Record<string, string> = {
  '/': 'Overview',
  '/jobs': 'Jobs',
  '/workers': 'Workers',
  '/settings': 'Settings',
}

function pageTitle(pathname: string): string {
  if (TITLES[pathname]) return TITLES[pathname]
  if (pathname.startsWith('/jobs/')) return 'Job detail'
  if (pathname.startsWith('/runs/')) return 'Run detail'
  return 'TaskFlow'
}

function Layout() {
  const location = useLocation()
  return (
    <div className="flex h-screen bg-surface">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar title={pageTitle(location.pathname)} />
        <main className="flex-1 overflow-y-auto p-6">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/jobs" element={<Jobs />} />
            <Route path="/jobs/:id" element={<JobDetail />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/workers" element={<Workers />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </div>
  )
}

function App() {
  const { isConfigured } = useConnection()
  return isConfigured ? <Layout /> : <ConnectScreen />
}

export default App
