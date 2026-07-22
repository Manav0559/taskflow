import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { onUnauthorized } from '@/lib/authEvents'

const TOKEN_KEY = 'taskflow.token'
const BASE_URL_KEY = 'taskflow.apiBaseUrl'
const PROM_URL_KEY = 'taskflow.prometheusUrl'
const DEFAULT_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_PROM_URL = import.meta.env.VITE_PROMETHEUS_URL ?? 'http://localhost:9093'

interface ConnectionState {
  token: string
  apiBaseUrl: string
  prometheusUrl: string
  isConfigured: boolean
  setConnection: (token: string, apiBaseUrl: string) => void
  setPrometheusUrl: (url: string) => void
  disconnect: () => void
}

const ConnectionContext = createContext<ConnectionState | null>(null)

export function ConnectionProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY) ?? '')
  const [apiBaseUrl, setApiBaseUrl] = useState(
    () => localStorage.getItem(BASE_URL_KEY) ?? DEFAULT_BASE_URL,
  )
  const [prometheusUrl, setPrometheusUrlState] = useState(
    () => localStorage.getItem(PROM_URL_KEY) ?? DEFAULT_PROM_URL,
  )

  const value = useMemo<ConnectionState>(
    () => ({
      token,
      apiBaseUrl,
      prometheusUrl,
      isConfigured: token.length > 0,
      setConnection: (newToken: string, newBaseUrl: string) => {
        localStorage.setItem(TOKEN_KEY, newToken)
        localStorage.setItem(BASE_URL_KEY, newBaseUrl)
        setToken(newToken)
        setApiBaseUrl(newBaseUrl)
      },
      setPrometheusUrl: (url: string) => {
        localStorage.setItem(PROM_URL_KEY, url)
        setPrometheusUrlState(url)
      },
      disconnect: () => {
        localStorage.removeItem(TOKEN_KEY)
        setToken('')
      },
    }),
    [token, apiBaseUrl, prometheusUrl],
  )

  useEffect(() => {
    onUnauthorized(() => {
      localStorage.removeItem(TOKEN_KEY)
      setToken('')
    })
  }, [])

  return <ConnectionContext.Provider value={value}>{children}</ConnectionContext.Provider>
}

export function useConnection(): ConnectionState {
  const ctx = useContext(ConnectionContext)
  if (!ctx) throw new Error('useConnection must be used within ConnectionProvider')
  return ctx
}
