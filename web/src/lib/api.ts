// Typed client for the argus admin API. Auth is cookie-based; mutating
// requests also attach the CSRF token returned by login via X-CSRF-Token.
export const CSRF_HEADER = "X-CSRF-Token"

let csrfToken: string | null = null

export function setCsrf(token: string | null): void {
  csrfToken = token
}

export class UnauthorizedError extends Error {
  constructor() {
    super("unauthorized")
    this.name = "UnauthorizedError"
  }
}

export class ApiError extends Error {
  status: number
  constructor(status: number) {
    super(`request failed (${status})`)
    this.name = "ApiError"
    this.status = status
  }
}

// --- Wire types (mirror pkg/metrics and pkg/server JSON). ---

export interface HostInfo {
  hostname: string
  os: string
  kernel: string
  arch: string
  cpu_model: string
  cpu_cores: number
  mem_total: number
  public_ip?: string
  agent_version: string
}

export interface NodeSummary {
  node_id: string
  hostname: string
  first_seen: number
  last_seen: number
}

export interface NodeDetail {
  node_id: string
  first_seen: number
  last_seen: number
  host_info?: HostInfo
}

export interface SeriesPoint {
  ts: number
  cpu: number
  cpu_peak: number
  load1: number
  load5: number
  load15: number
  mem_total: number
  mem_avail: number
  mem_used: number
  swap_total: number
  swap_used: number
  net_rx_rate: number
  net_tx_rate: number
  disk_read_rate: number
  disk_write_rate: number
  disk_total?: number
  disk_used?: number
  logged_users?: number
}

export interface SeriesResponse {
  resolution: string
  points: SeriesPoint[]
}

export interface Sample {
  ts: number
  cpu: { used_percent: number }
  load: { load1: number; load5: number; load15: number }
  mem: { total_bytes: number; available_bytes: number; used_bytes: number }
  swap: { total_bytes: number; used_bytes: number }
  net: { rx_bytes: number; tx_bytes: number }
  disk_io: { read_bytes: number; write_bytes: number }
  disk_cap?: { total_bytes: number; used_bytes: number }
  logged_users?: number
}

interface MeResponse {
  username: string
}

interface LoginResponse {
  csrf_token: string
}

export interface CreateNodeResponse {
  node_id: string
  token: string
}

async function request<T>(
  path: string,
  init: { method?: string; body?: unknown } = {},
): Promise<T> {
  const method = init.method ?? "GET"
  const headers: Record<string, string> = {}
  let body: string | undefined
  if (init.body !== undefined) {
    headers["Content-Type"] = "application/json"
    body = JSON.stringify(init.body)
  }
  if (method !== "GET" && csrfToken) {
    headers[CSRF_HEADER] = csrfToken
  }

  const res = await fetch(path, {
    method,
    headers,
    body,
    credentials: "same-origin",
  })

  if (res.status === 204) return undefined as T
  if (res.status === 401) throw new UnauthorizedError()

  const data: unknown = await res.json().catch(() => undefined)
  if (!res.ok) throw new ApiError(res.status)
  return data as T
}

export const api = {
  login: (username: string, password: string) =>
    request<LoginResponse>("/api/v1/auth/login", {
      method: "POST",
      body: { username, password },
    }),
  logout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),
  me: () => request<MeResponse>("/api/v1/auth/me"),
  listNodes: () => request<NodeSummary[]>("/api/v1/nodes"),
  getNode: (id: string) =>
    request<NodeDetail>(`/api/v1/nodes/${encodeURIComponent(id)}`),
  createNode: (nodeId: string) =>
    request<CreateNodeResponse>("/api/v1/nodes", {
      method: "POST",
      body: { node_id: nodeId },
    }),
  deleteNode: (nodeId: string) =>
    request<void>("/api/v1/nodes", {
      method: "DELETE",
      body: { node_id: nodeId },
    }),
  series: (id: string, from: number, to: number) =>
    request<SeriesResponse>(
      `/api/v1/nodes/${encodeURIComponent(id)}/series?from=${from}&to=${to}`,
    ),
}
