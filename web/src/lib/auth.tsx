import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"
import type { ReactNode } from "react"
import { api, setCsrf } from "@/lib/api"

interface AuthContextValue {
  username: string | null
  setupRequired: boolean
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  setup: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [username, setUsername] = useState<string | null>(null)
  const [setupRequired, setSetupRequired] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      // Check first-run state before /me: on a fresh database /me returns 401
      // and the app must show the setup wizard instead of the login page.
      let required = false
      try {
        required = (await api.setupStatus()).setup_required
      } catch {
        // Older servers have no setup endpoint; assume they are configured.
      }
      if (cancelled) return
      setSetupRequired(required)
      if (!required) {
        try {
          const u = await api.me()
          if (cancelled) return
          setUsername(u.username)
          setCsrf(u.csrf_token)
        } catch {
          if (!cancelled) setUsername(null)
        }
      }
      if (!cancelled) setLoading(false)
    }
    load()
    return () => {
      cancelled = true
    }
  }, [])

  const login = useCallback(async (u: string, p: string) => {
    const resp = await api.login(u, p)
    setCsrf(resp.csrf_token)
    const me = await api.me()
    setUsername(me.username)
    setCsrf(me.csrf_token)
  }, [])

  const setup = useCallback(async (u: string, p: string) => {
    const resp = await api.setup(u, p)
    setCsrf(resp.csrf_token)
    setUsername(resp.username)
    setSetupRequired(false)
  }, [])

  const logout = useCallback(async () => {
    await api.logout().catch(() => {})
    setCsrf(null)
    setUsername(null)
  }, [])

  const value = useMemo(
    () => ({ username, setupRequired, loading, login, setup, logout }),
    [username, setupRequired, loading, login, setup, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
