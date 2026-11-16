import { useEffect, useState } from "react"
import type { FormEvent } from "react"
import { Copy, Plus, Trash2 } from "lucide-react"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { api, type NodeSummary } from "@/lib/api"
import { lastSeenAgo } from "@/lib/format"

// ConfirmAction is a pending confirmation shown in the AlertDialog instead of a
// browser-native window.confirm popup.
type ConfirmAction = {
  title: string
  description: string
  confirmLabel: string
  destructive?: boolean
  onConfirm: () => void
}

export function SettingsPage() {
  const [nodes, setNodes] = useState<NodeSummary[] | null>(null)
  const [newId, setNewId] = useState("")
  const [newTags, setNewTags] = useState("")
  const [created, setCreated] = useState<{
    node_id: string
    token: string
    rotated?: boolean
  } | null>(null)
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copiedCmd, setCopiedCmd] = useState(false)

  const refresh = () => {
    api
      .listNodes()
      .then(setNodes)
      .catch(() => setNodes([]))
  }
  useEffect(() => {
    refresh()
  }, [])

  const create = async (e: FormEvent) => {
    e.preventDefault()
    if (!newId.trim()) return
    setBusy(true)
    setError(null)
    setCreated(null)
    try {
      const tags = newTags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean)
      const resp = await api.createNode(newId.trim(), tags)
      setCreated(resp)
      setNewId("")
      setNewTags("")
      refresh()
    } catch {
      setError("Could not create node")
    } finally {
      setBusy(false)
    }
  }

  const remove = (id: string) => {
    setConfirm({
      title: `Delete node “${id}”?`,
      description:
        "Historical samples are retained until they expire. The agent on that host will be rejected until it is reinstalled with a new token.",
      confirmLabel: "Delete",
      destructive: true,
      onConfirm: async () => {
        try {
          await api.deleteNode(id)
          refresh()
        } catch {
          // ignore; the list refresh reflects server state
        }
      },
    })
  }

  const copyToken = async () => {
    if (!created) return
    try {
      await navigator.clipboard.writeText(created.token)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard may be unavailable; the user can select the text
    }
  }

  // rotate issues a fresh token for an existing node, which is the only way to
  // show its install command again: the server stores just the token hash. The
  // previous token stops working, so ask for confirmation first.
  const rotate = (id: string) => {
    setConfirm({
      title: `Generate a new token for “${id}”?`,
      description:
        "The current token stops working immediately: the agent on that host will report 401 until you reinstall it with the new install command.",
      confirmLabel: "Generate token",
      onConfirm: async () => {
        setActionError(null)
        try {
          const resp = await api.rotateNodeToken(id)
          setCreated({ ...resp, rotated: true })
          setCopied(false)
          setCopiedCmd(false)
        } catch {
          setActionError("Could not generate a new token")
        }
      },
    })
  }

  const curl = created
    ? `curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh | sudo bash -s -- --server '${window.location.origin}' --node-id '${created.node_id}' --token '${created.token}'`
    : ""

  const copyCommand = async () => {
    if (!curl) return
    try {
      await navigator.clipboard.writeText(curl)
      setCopiedCmd(true)
      setTimeout(() => setCopiedCmd(false), 2000)
    } catch {
      // clipboard may be unavailable; the user can select the text
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-lg font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Manage registered agents.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Add node</CardTitle>
          <CardDescription>
            Generates a node ID and one-time secret token.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={create}
            className="flex flex-col gap-3 sm:flex-row sm:items-end"
          >
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <Label htmlFor="node-id">Node ID</Label>
              <Input
                id="node-id"
                value={newId}
                onChange={(e) => setNewId(e.target.value)}
                placeholder="us-west-01"
              />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <Label htmlFor="node-tags">Tags</Label>
              <Input
                id="node-tags"
                value={newTags}
                onChange={(e) => setNewTags(e.target.value)}
                placeholder="prod, web (optional)"
              />
            </div>
            <Button
              type="submit"
              className="w-full sm:w-auto"
              disabled={busy || !newId.trim()}
            >
              <Plus />
              Add
            </Button>
          </form>
          {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Nodes</CardTitle>
          <CardDescription>{nodes?.length ?? 0} registered</CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          {created && (
            <div className="mx-6 mb-4 rounded-lg border border-border bg-muted/40 p-4 text-sm">
              <p className="font-medium">
                {created.rotated
                  ? `New token for node “${created.node_id}”. The previous token no longer works; copy this one now (it cannot be shown again), then reinstall the agent with:`
                  : `Node “${created.node_id}” created. Copy the token now — it is shown only once.`}
              </p>
              <div className="mt-3 flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded bg-background px-2 py-1 font-mono text-xs">
                  {created.token}
                </code>
                <Button variant="outline" size="sm" onClick={copyToken}>
                  <Copy />
                  {copied ? "Copied" : "Copy"}
                </Button>
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                Install an agent with:
              </p>
              <div className="mt-1 flex items-start gap-2">
                <code className="min-w-0 flex-1 overflow-x-auto rounded bg-background px-2 py-1 font-mono text-[11px] whitespace-pre">
                  {curl}
                </code>
                <Button variant="outline" size="sm" onClick={copyCommand}>
                  <Copy />
                  {copiedCmd ? "Copied" : "Copy"}
                </Button>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                This command contains the node token, which grants write access
                to this node&apos;s metrics. Treat it like a password; it may be
                recorded in the target host&apos;s shell history.
              </p>
            </div>
          )}
          {actionError && (
            <p className="mx-6 mb-2 text-xs text-destructive">{actionError}</p>
          )}
          {nodes === null ? (
            <p className="px-6 text-sm text-muted-foreground">Loading…</p>
          ) : nodes.length === 0 ? (
            <p className="px-6 text-sm text-muted-foreground">No nodes.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-160 text-sm">
                <thead>
                  <tr className="border-b border-border text-left text-xs text-muted-foreground">
                    <th className="px-6 py-2 font-medium">Node</th>
                    <th className="px-6 py-2 font-medium">Hostname</th>
                    <th className="px-6 py-2 font-medium">Tags</th>
                    <th className="px-6 py-2 font-medium">Last seen</th>
                    <th className="px-6 py-2" />
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((n) => (
                    <tr
                      key={n.node_id}
                      className="border-b border-border/50 last:border-0"
                    >
                      <td className="px-6 py-2 font-mono text-xs">
                        {n.node_id}
                      </td>
                      <td className="px-6 py-2">{n.hostname || "—"}</td>
                      <td className="px-6 py-2 text-muted-foreground">
                        {n.tags?.length ? n.tags.join(", ") : "—"}
                      </td>
                      <td className="px-6 py-2 text-muted-foreground">
                        {lastSeenAgo(n.last_seen)}
                      </td>
                      <td className="px-6 py-2 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => rotate(n.node_id)}
                          >
                            Install
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => remove(n.node_id)}
                            aria-label={`Delete ${n.node_id}`}
                          >
                            <Trash2 />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => {
          if (!open) setConfirm(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirm?.title}</AlertDialogTitle>
            <AlertDialogDescription>
              {confirm?.description}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={confirm?.onConfirm}
              className={
                confirm?.destructive
                  ? buttonVariants({ variant: "destructive" })
                  : undefined
              }
            >
              {confirm?.confirmLabel}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
