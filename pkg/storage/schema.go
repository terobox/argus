package storage

import "fmt"

// rawColumns lists the metric columns of samples_raw in the order used by every
// INSERT and SELECT. node_id is handled separately.
const rawColumns = "ts, cpu_used, load1, load5, load15, mem_total, mem_avail, mem_used, swap_total, swap_used, net_rx, net_tx, disk_r, disk_w, disk_total, disk_used, logged_users"

// migrations is the ordered list of DDL statements applied on startup. Later
// schema changes append new statements here so existing databases upgrade in
// place rather than requiring a drop-and-recreate.
var migrations = []string{
	schemaNodes,
	schemaRaw,
	fmt.Sprintf(schemaAgg, "samples_15s"),
	fmt.Sprintf(schemaAgg, "samples_5m"),
	schemaAdmins,
	schemaSessions,
	schemaNodeInfo,
	schemaAlertState,
}

// schemaNodes stores the registered agents and their hashed tokens. The
// plaintext token is never persisted; only its SHA-256 digest (see HashToken) is
// kept, so a database leak does not expose usable agent credentials.
const schemaNodes = `
CREATE TABLE IF NOT EXISTS nodes (
    node_id    TEXT    PRIMARY KEY,
    token_hash TEXT    NOT NULL,
    first_seen INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL,
    tags       TEXT    NOT NULL DEFAULT ''
);
`

// samples_raw stores the hot, fine-grained (1s) window. WITHOUT ROWID makes the
// composite PRIMARY KEY (node_id, ts) the clustered index, which is exactly the
// access pattern for time-series: ordered range scans per node, and no extra
// index overhead. Retention for this table is ~1h.
const schemaRaw = `
CREATE TABLE IF NOT EXISTS samples_raw (
    node_id      TEXT    NOT NULL,
    ts           INTEGER NOT NULL,
    cpu_used     REAL    NOT NULL,
    load1        REAL    NOT NULL,
    load5        REAL    NOT NULL,
    load15       REAL    NOT NULL,
    mem_total    INTEGER NOT NULL,
    mem_avail    INTEGER NOT NULL,
    mem_used     INTEGER NOT NULL,
    swap_total   INTEGER NOT NULL,
    swap_used    INTEGER NOT NULL,
    net_rx       INTEGER NOT NULL,
    net_tx       INTEGER NOT NULL,
    disk_r       INTEGER NOT NULL,
    disk_w       INTEGER NOT NULL,
    disk_total   INTEGER,
    disk_used    INTEGER,
    logged_users INTEGER,
    PRIMARY KEY (node_id, ts)
) WITHOUT ROWID;
`

// schemaAgg is the schema for the downsampled tables (samples_15s, samples_5m).
// They differ from samples_raw only in the CPU columns: cpu_used becomes an
// average (cpu_avg) plus a peak (cpu_peak) so a coarse bucket does not hide a
// transient spike.
const schemaAgg = `
CREATE TABLE IF NOT EXISTS %s (
    node_id      TEXT    NOT NULL,
    ts           INTEGER NOT NULL,
    cpu_avg      REAL    NOT NULL,
    cpu_peak     REAL    NOT NULL,
    load1        REAL    NOT NULL,
    load5        REAL    NOT NULL,
    load15       REAL    NOT NULL,
    mem_total    INTEGER NOT NULL,
    mem_avail    INTEGER NOT NULL,
    mem_used     INTEGER NOT NULL,
    swap_total   INTEGER NOT NULL,
    swap_used    INTEGER NOT NULL,
    net_rx       INTEGER NOT NULL,
    net_tx       INTEGER NOT NULL,
    disk_r       INTEGER NOT NULL,
    disk_w       INTEGER NOT NULL,
    disk_total   INTEGER,
    disk_used    INTEGER,
    logged_users INTEGER,
    PRIMARY KEY (node_id, ts)
) WITHOUT ROWID;
`

// schemaAdmins stores the dashboard administrator(s). PasswordHash is a bcrypt
// digest (see pkg/auth.HashPassword); the plaintext password is never persisted.
const schemaAdmins = `
CREATE TABLE IF NOT EXISTS admins (
    username      TEXT    PRIMARY KEY,
    password_hash TEXT    NOT NULL,
    created_at    INTEGER NOT NULL
);
`

// schemaSessions stores login sessions. token_hash is the SHA-256 digest of the
// session cookie value, so a database leak cannot forge a session cookie. The
// csrf_token is a separate random value returned to the client and required on
// mutating requests.
const schemaSessions = `
CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT    PRIMARY KEY,
    username   TEXT    NOT NULL,
    csrf_token TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
`

// schemaAlertState stores the alert evaluator's per-(node, rule) firing state and
// last-notified time, so a restart neither re-fires a reported transition nor
// resets the cooldown.
const schemaAlertState = `
CREATE TABLE IF NOT EXISTS alert_state (
    node_id       TEXT    NOT NULL,
    rule          TEXT    NOT NULL,
    firing        INTEGER NOT NULL,
    last_notified INTEGER NOT NULL,
    PRIMARY KEY (node_id, rule)
) WITHOUT ROWID;
`

// schemaNodeInfo stores each host's mostly-static identifying information,
// reported by the agent on startup. It is keyed by node_id and updated in place
// so a re-report (or host change) never duplicates rows.
const schemaNodeInfo = `
CREATE TABLE IF NOT EXISTS node_info (
    node_id    TEXT    PRIMARY KEY,
    hostname   TEXT    NOT NULL,
    os         TEXT    NOT NULL,
    kernel     TEXT    NOT NULL,
    arch       TEXT    NOT NULL,
    cpu_model  TEXT    NOT NULL,
    cpu_cores  INTEGER NOT NULL,
    mem_total  INTEGER NOT NULL,
    public_ip  TEXT    NOT NULL,
    agent_ver  TEXT    NOT NULL,
    updated_at INTEGER NOT NULL
);
`
