import { useEffect, useState } from "react"
import type { Sample } from "@/lib/api"

export interface NodeMetrics {
  cpu_percent: number
  mem_used: number
  mem_total: number
  disk_used: number
  disk_total: number
  net_rx_rate: number
  net_tx_rate: number
}

export interface StatusEvent {
  node_id: string
  hostname: string
  agent_version: string
  first_seen: number
  last_seen: number
  online: boolean
  cpu_cores: number
  tags: string[]
  live?: NodeMetrics
}

export interface SampleEvent {
  node_id: string
  sample: Sample
}

// useNodesStream subscribes to the server's SSE status events and returns the
// latest node list (null until the first event) plus a connection flag.
export function useNodesStream() {
  const [nodes, setNodes] = useState<StatusEvent[] | null>(null)
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const es = new EventSource("/api/v1/stream")
    const onStatus = (e: Event) => {
      const msg = e as MessageEvent
      setNodes(JSON.parse(msg.data) as StatusEvent[])
      setConnected(true)
    }
    es.addEventListener("status", onStatus)
    es.onerror = () => setConnected(false)
    return () => es.close()
  }, [])

  return { nodes, connected }
}

// useSampleStream subscribes to live sample events, filtering to one node.
export function useSampleStream(nodeId: string) {
  const [sample, setSample] = useState<Sample | null>(null)
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const es = new EventSource("/api/v1/stream")
    const onSample = (e: Event) => {
      const msg = e as MessageEvent
      const ev = JSON.parse(msg.data) as SampleEvent
      if (ev.node_id === nodeId) {
        setSample(ev.sample)
        setConnected(true)
      }
    }
    es.addEventListener("sample", onSample)
    es.onerror = () => setConnected(false)
    return () => es.close()
  }, [nodeId])

  return { sample, connected }
}
