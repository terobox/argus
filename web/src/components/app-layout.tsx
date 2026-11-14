import { NavLink, Outlet, useNavigate } from "react-router-dom"
import { LayoutDashboard, LogOut, Moon, Settings, Sun } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth"
import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

function navCls({ isActive }: { isActive: boolean }) {
  return cn(
    "flex items-center gap-2 rounded-none px-2 py-1.5 text-sm transition-colors",
    isActive
      ? "bg-sidebar-accent text-sidebar-accent-foreground"
      : "hover:bg-sidebar-accent/60",
  )
}

export function AppLayout() {
  const { username, logout } = useAuth()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate("/login")
  }

  return (
    <div className="flex min-h-svh">
      <aside className="flex w-52 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
        <div className="flex h-14 items-center gap-2 px-4 text-sm font-semibold">
          <span className="size-2 rounded-full bg-sidebar-primary" />
          Argus
        </div>
        <nav className="flex flex-1 flex-col gap-1 px-2 py-2">
          <NavLink to="/" end className={navCls}>
            <LayoutDashboard className="size-4" />
            Overview
          </NavLink>
          <NavLink to="/settings" className={navCls}>
            <Settings className="size-4" />
            Settings
          </NavLink>
        </nav>
        <div className="flex items-center gap-1 border-t px-2 py-2">
          <span className="min-w-0 flex-1 truncate px-1 text-xs text-sidebar-foreground/70">
            {username}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
            aria-label="Toggle theme"
          >
            {theme === "dark" ? <Sun /> : <Moon />}
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={handleLogout}
            aria-label="Log out"
          >
            <LogOut />
          </Button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 p-6">
        <Outlet />
      </main>
    </div>
  )
}
