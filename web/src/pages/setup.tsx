import { useState } from "react"
import type { FormEvent } from "react"
import { Navigate, useNavigate } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { ApiError } from "@/lib/api"
import { useAuth } from "@/lib/auth"

const MIN_PASSWORD_LENGTH = 8

export function SetupPage() {
  const { username, setupRequired, loading, setup } = useAuth()
  const navigate = useNavigate()
  const [user, setUser] = useState("admin")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    )
  }
  if (!setupRequired) return <Navigate to={username ? "/" : "/login"} replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const name = user.trim()
    if (!name) {
      setError("Username is required")
      return
    }
    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
      return
    }
    setBusy(true)
    setError(null)
    try {
      await setup(name, password)
      navigate("/")
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError("Setup has already been completed. Refresh the page.")
      } else if (err instanceof ApiError && err.status === 400) {
        setError("Please check the username and password.")
      } else {
        setError("Could not create the administrator. Try again.")
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Welcome to Argus</CardTitle>
          <CardDescription>
            Create the administrator account to finish setting up this server.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                value={user}
                onChange={(e) => setUser(e.target.value)}
                autoComplete="username"
                required
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                minLength={MIN_PASSWORD_LENGTH}
                required
              />
              <p className="text-xs text-muted-foreground">
                At least {MIN_PASSWORD_LENGTH} characters.
              </p>
            </div>
            {error && <p className="text-xs text-destructive">{error}</p>}
            <Button type="submit" disabled={busy} className="w-full">
              {busy ? "Creating…" : "Create administrator"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
