package protocol

// IngestPath is the agent's sample-ingest endpoint on the server. The agent and
// server share this constant so the route cannot drift out of sync.
const IngestPath = "/api/v1/report"

// HostPath is the agent's host-info ingest endpoint.
const HostPath = "/api/v1/host"

// NodeTokenHeader is the request header carrying the agent's node token. The
// server compares the plaintext token against a stored SHA-256 hash.
const NodeTokenHeader = "X-Node-Token"

// Admin API endpoints used by the web UI. The agent does not call these; they
// are shared constants so the server's routes and the web UI cannot drift out
// of sync.
const (
	LoginPath  = "/api/v1/auth/login"
	LogoutPath = "/api/v1/auth/logout"
	MePath     = "/api/v1/auth/me"
	NodesPath  = "/api/v1/nodes"

	// SetupPath creates the first admin when the database has none, and
	// SetupStatusPath reports whether that is still possible. Both are public
	// because they must work before any account exists.
	SetupPath       = "/api/v1/setup"
	SetupStatusPath = "/api/v1/setup/status"

	// NodeDetailPath is the mux pattern for a single node's detail (host info);
	// {node_id} is a path value.
	NodeDetailPath = "/api/v1/nodes/{node_id}"

	// NodeTokenPath rotates a node's token and returns the new one; {node_id}
	// is a path value. Only the hash is stored, so this is the only way to get
	// an install command for a node that already exists.
	NodeTokenPath = "/api/v1/nodes/{node_id}/token"

	// StreamPath is the SSE endpoint for live node/sample updates.
	StreamPath = "/api/v1/stream"

	// SeriesPath is the mux pattern for a node's metric series; {node_id} is a
	// path value.
	SeriesPath = "/api/v1/nodes/{node_id}/series"
)
