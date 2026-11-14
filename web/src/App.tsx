import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom"
import type { ReactNode } from "react"
import { AuthProvider, useAuth } from "@/lib/auth"
import { AppLayout } from "@/components/app-layout"
import { LoginPage } from "@/pages/login"
import { OverviewPage } from "@/pages/overview"
import { NodePage } from "@/pages/node"
import { SettingsPage } from "@/pages/settings"

function RequireAuth({ children }: { children: ReactNode }) {
  const { username, loading } = useAuth()
  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    )
  }
  if (!username) return <Navigate to="/login" replace />
  return <>{children}</>
}

export function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            element={
              <RequireAuth>
                <AppLayout />
              </RequireAuth>
            }
          >
            <Route path="/" element={<OverviewPage />} />
            <Route path="/nodes/:id" element={<NodePage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}

export default App
