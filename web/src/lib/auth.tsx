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
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [username, setUsername] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    api
      .me()
      .then((u) => {
        if (!cancelled) {
          setUsername(u.username)
          setCsrf(u.csrf_token)
        }
      })
      .catch(() => {
        if (!cancelled) setUsername(null)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
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

  const logout = useCallback(async () => {
    await api.logout().catch(() => {})
    setCsrf(null)
    setUsername(null)
  }, [])

  const value = useMemo(
    () => ({ username, loading, login, logout }),
    [username, loading, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
