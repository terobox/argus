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
      : "hover:bg-sidebar-accent/60"
  )
}

// Logo is the graph-star mark from _notes/design/assets/graph-star.svg, inlined
// so it inherits currentColor and adapts to light/dark theme automatically.
function Logo({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
    >
      <path d="m12.342 9.316 1.764-3.527" />
      <path d="m12.59 14.545 2.35 3.759" />
      <path d="m13.982 12.332 4.03.447" />
      <path d="M5.414 17.586 8.88 14.12" />
      <path d="M9.025 9.742 5.317 5.505" />
      <circle cx="11" cy="12" r="3" />
      <circle cx="15" cy="4" r="2" />
      <circle cx="16" cy="20" r="2" />
      <circle cx="20" cy="13" r="2" />
      <circle cx="4" cy="19" r="2" />
      <circle cx="4" cy="4" r="2" />
    </svg>
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
    <div className="flex h-svh flex-col overflow-hidden md:flex-row">
      {/* Below md the sidebar is replaced by this compact top bar; the desktop
          sidebar and content area are unchanged. */}
      <header className="flex h-12 shrink-0 items-center gap-1 border-b bg-sidebar px-2 text-sidebar-foreground md:hidden">
        <span className="flex items-center gap-2 px-2 text-sm font-semibold">
          <Logo className="size-4 text-sidebar-primary" />
          Argus
        </span>
        <nav className="ml-auto flex items-center gap-0.5">
          <NavLink
            to="/"
            end
            aria-label="Overview"
            className={({ isActive }) =>
              cn(
                "inline-flex size-8 items-center justify-center transition-colors",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "hover:bg-sidebar-accent/60"
              )
            }
          >
            <LayoutDashboard className="size-4" />
          </NavLink>
          <NavLink
            to="/settings"
            aria-label="Settings"
            className={({ isActive }) =>
              cn(
                "inline-flex size-8 items-center justify-center transition-colors",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "hover:bg-sidebar-accent/60"
              )
            }
          >
            <Settings className="size-4" />
          </NavLink>
        </nav>
        <div className="flex items-center gap-1 border-l pl-1">
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
      </header>
      <aside className="hidden w-52 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground md:flex">
        <div className="flex h-14 items-center gap-2 px-4 text-sm font-semibold">
          <Logo className="size-4 text-sidebar-primary" />
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
      <main className="min-w-0 flex-1 overflow-y-auto p-4 md:p-6">
        <Outlet />
      </main>
    </div>
  )
}
