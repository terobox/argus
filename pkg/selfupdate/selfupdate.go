// Package selfupdate downloads the latest release of a single binary asset from
// GitHub Releases and installs it atomically over the running executable,
// verifying the asset's SHA-256 against the release's checksums.txt. It is used
// by argus-agent only: argus-server is upgraded by the deployment tool (shipit).
// It uses only the standard library, so the agent needs no extra dependency to
// upgrade itself.
//
// Unauthenticated GitHub API requests are rate-limited to 60/hour per IP; a
// self-update check happens rarely, so that is ample.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Updater fetches releases of one asset from one repository.
type Updater struct {
	repo      string // "owner/name"
	assetName string // exact release asset name (raw binary, see scripts/package.sh)
	current   string // running version; "dev" is always treated as out of date

	apiBase string // GitHub API base URL, injectable for tests
	client  *http.Client
}

// New returns an Updater for the release asset named assetName in repo.
func New(repo, assetName, current string) *Updater {
	return &Updater{
		repo:      repo,
		assetName: assetName,
		current:   current,
		apiBase:   "https://api.github.com",
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Release is the metadata needed to download a release's assets.
type Release struct {
	// Tag is the git tag, e.g. "v1.2.3".
	Tag string
	// assets maps asset name to download URL.
	assets map[string]string
}

// Asset returns the download URL for the named asset, or ok=false if the release
// has no such asset.
func (r Release) Asset(name string) (string, bool) {
	url, ok := r.assets[name]
	return url, ok
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Latest fetches the newest non-prerelease, non-draft release metadata.
func (u *Updater) Latest(ctx context.Context) (Release, error) {
	url := u.apiBase + "/repos/" + u.repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "argus-selfupdate")

	resp, err := u.client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("selfupdate: fetch release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return Release{}, fmt.Errorf("selfupdate: fetch release: %s", resp.Status)
	}

	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return Release{}, fmt.Errorf("selfupdate: decode release: %w", err)
	}
	rel := Release{Tag: gr.TagName, assets: make(map[string]string, len(gr.Assets))}
	for _, a := range gr.Assets {
		rel.assets[a.Name] = a.BrowserDownloadURL
	}
	return rel, nil
}

// Update fetches the latest release and, if newer than u.current, downloads the
// asset, verifies its checksum, and atomically installs it over execPath. It
// returns the resulting version and whether a replacement was applied.
func (u *Updater) Update(ctx context.Context, execPath string) (version string, applied bool, err error) {
	rel, err := u.Latest(ctx)
	if err != nil {
		return "", false, err
	}
	if sameVersion(rel.Tag, u.current) {
		return rel.Tag, false, nil
	}

	assetURL, ok := rel.Asset(u.assetName)
	if !ok {
		return "", false, fmt.Errorf("selfupdate: release %s has no asset %q", rel.Tag, u.assetName)
	}
	checksumURL, ok := rel.Asset("checksums.txt")
	if !ok {
		return "", false, fmt.Errorf("selfupdate: release %s has no checksums.txt", rel.Tag)
	}

	want, err := u.fetchChecksum(ctx, checksumURL, u.assetName)
	if err != nil {
		return "", false, err
	}
	if err := u.install(ctx, execPath, assetURL, want); err != nil {
		return "", false, err
	}
	return rel.Tag, true, nil
}

// fetchChecksum downloads the release checksums.txt and returns the lowercase
// hex SHA-256 recorded for assetName.
func (u *Updater) fetchChecksum(ctx context.Context, url, assetName string) (string, error) {
	data, err := u.download(ctx, url)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && filepath.Base(fields[1]) == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("selfupdate: checksums.txt has no entry for %q", assetName)
}

// download fetches a small resource (checksums.txt) into memory.
func (u *Updater) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "argus-selfupdate")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("selfupdate: download: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// install streams the asset into a temp file in the same directory as execPath,
// verifies its checksum, and renames it over execPath. Writing next to the target
// keeps the final rename atomic (same filesystem), so a crash mid-update can
// never leave a truncated binary at the target path.
func (u *Updater) install(ctx context.Context, execPath, assetURL, wantSHA string) error {
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".argus-selfupdate-*.tmp")
	if err != nil {
		return fmt.Errorf("selfupdate: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed into place

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		tmp.Close()
		return err
	}
	req.Header.Set("User-Agent", "argus-selfupdate")
	resp, err := u.client.Do(req)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("selfupdate: download asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("selfupdate: download asset: %s", resp.Status)
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, 128<<20)); err != nil {
		tmp.Close()
		return fmt.Errorf("selfupdate: download asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("selfupdate: write asset: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(got), []byte(wantSHA)) != 1 {
		return fmt.Errorf("selfupdate: checksum mismatch: got %s want %s", got, wantSHA)
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("selfupdate: chmod: %w", err)
	}
	if err := os.Rename(tmpName, execPath); err != nil {
		return fmt.Errorf("selfupdate: rename: %w", err)
	}
	return nil
}

// sameVersion reports whether two version strings denote the same release,
// ignoring a leading "v" so a build stamped "1.2.3" matches a tag "v1.2.3".
func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}
