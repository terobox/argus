# Metrics Dictionary

This document defines every metric argus collects, its unit, source, collection
cadence, and how it is aggregated during downsampling. Trust in a monitoring
tool is trust in its numbers; keep this file and the collector implementations
in lockstep.

## Conventions

- Time is unix seconds, from the agent's clock.
- Byte counters (`net`, `disk_io`) are **cumulative since boot**. Rates (bytes/s)
  are derived server-side as `delta(counter) / delta(ts)`; this is robust to a
  missed tick, unlike agent-side rates.
- All "used" memory figures are derived from `MemAvailable` where relevant, so
  they reflect reclaimable page cache rather than a misleadingly low `free`.
- Downsample aggregation per metric type:
  - **gauge** → bucket average (with peak retained where noted)
  - **counter** → last cumulative value in the bucket; rates are derived at read
    time from consecutive buckets, so the model stays order-independent under
    backfill
  - **capacity** → last value in bucket (a capacity is near-constant)

## Fast metrics (collected every tick, default 1s)

| Metric | Unit | Source (Linux) | Type | Downsample |
| --- | --- | --- | --- | --- |
| `cpu.used_percent` | % (0–100) | `/proc/stat` delta | gauge | avg + peak |
| `load1` / `load5` / `load15` | — | `/proc/loadavg` | gauge | avg |
| `mem.total_bytes` | bytes | `/proc/meminfo` `MemTotal` | capacity | last |
| `mem.available_bytes` | bytes | `/proc/meminfo` `MemAvailable` | gauge | avg |
| `mem.used_bytes` | bytes | `MemTotal - MemAvailable` | gauge | avg |
| `swap.total_bytes` | bytes | `/proc/meminfo` `SwapTotal` | capacity | last |
| `swap.used_bytes` | bytes | `SwapTotal - SwapFree` | gauge | avg |
| `net.rx_bytes` / `net.tx_bytes` | bytes | `/proc/net/dev` (primary NIC) | counter | last cumulative |
| `disk_io.read_bytes` / `disk_io.write_bytes` | bytes | `/proc/diskstats` (root device) | counter | last cumulative |

## Slow metrics (collected every 30s)

| Metric | Unit | Source | Type | Downsample |
| --- | --- | --- | --- | --- |
| `disk_cap.total_bytes` | bytes | `statfs("/")` | capacity | last |
| `disk_cap.used_bytes` | bytes | `statfs("/")` | capacity | last |
| `logged_users` | count | `/var/run/utmp` (`who`) | gauge | last |

## Static info (registration + rare refresh)

| Field | Source | Notes |
| --- | --- | --- |
| `hostname` | `os.Hostname()` | |
| `os` / `kernel` | `/etc/os-release`, `uname` | Ubuntu target |
| `arch` | `runtime.GOARCH` | |
| `cpu_model` / `cpu_cores` | `/proc/cpuinfo` | |
| `mem_total` | `/proc/meminfo` `MemTotal` | duplicate of fast metric, kept for identity |
| `public_ip` | manual config | never auto-probed (privacy) |
| `agent_version` | build-time const | |

## Primary interface / device selection

- **Network**: the interface carrying the default route (read from
  `/proc/net/route`). Counting only this NIC avoids double-counting from `lo`,
  bridges, and containers.
- **Disk**: the device backing `/` (from `/proc/mounts`), not `snap` loopbacks
  or overlay mounts.
