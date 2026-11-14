package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fakeGitHub serves a synthetic GitHub API at /repos/o/r/releases/latest plus
// the release assets under /assets/<name>.
func fakeGitHub(t *testing.T, tag string, assets map[string][]byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r/releases/latest":
			rel := struct {
				TagName string `json:"tag_name"`
				Assets  []struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				} `json:"assets"`
			}{TagName: tag}
			for name := range assets {
				rel.Assets = append(rel.Assets, struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				}{name, srv.URL + "/assets/" + name})
			}
			b, _ := json.Marshal(rel)
			w.Write(b)
		default:
			name := r.URL.Path[len("/assets/"):]
			if data, ok := assets[name]; ok {
				w.Write(data)
			} else {
				http.NotFound(w, r)
			}
		}
	}))
	return srv
}

func TestLatest(t *testing.T) {
	srv := fakeGitHub(t, "v1.2.3", map[string][]byte{
		"argus-agent_linux_amd64": []byte("bin"),
		"checksums.txt":           []byte("deadbeef  argus-agent_linux_amd64\n"),
	})
	defer srv.Close()

	u := New("o/r", "argus-agent_linux_amd64", "dev")
	u.apiBase = srv.URL
	rel, err := u.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Tag != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", rel.Tag)
	}
	if _, ok := rel.Asset("argus-agent_linux_amd64"); !ok {
		t.Fatal("asset not found")
	}
}

func TestUpdateApplies(t *testing.T) {
	asset := []byte("new-binary-v2")
	srv := fakeGitHub(t, "v2.0.0", map[string][]byte{
		"argus-agent_linux_amd64": asset,
		"checksums.txt":           []byte(sha256hex(asset) + "  argus-agent_linux_amd64\n"),
	})
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "argus-agent")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := New("o/r", "argus-agent_linux_amd64", "v1.0.0")
	u.apiBase = srv.URL
	version, applied, err := u.Update(context.Background(), exe)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !applied {
		t.Fatal("applied = false, want true")
	}
	if version != "v2.0.0" {
		t.Fatalf("version = %q, want v2.0.0", version)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(asset) {
		t.Fatalf("file = %q, want asset bytes", got)
	}
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatal("replaced binary is not executable")
	}
}

func TestUpdateUpToDate(t *testing.T) {
	asset := []byte("bin")
	srv := fakeGitHub(t, "v1.0.0", map[string][]byte{
		"argus-agent_linux_amd64": asset,
		"checksums.txt":           []byte(sha256hex(asset) + "  argus-agent_linux_amd64\n"),
	})
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "argus-agent")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Stamped "1.0.0" (no "v") matches tag "v1.0.0".
	u := New("o/r", "argus-agent_linux_amd64", "1.0.0")
	u.apiBase = srv.URL
	version, applied, err := u.Update(context.Background(), exe)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if applied {
		t.Fatal("applied = true, want false when up to date")
	}
	if version != "v1.0.0" {
		t.Fatalf("version = %q", version)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "old" {
		t.Fatal("file changed despite being up to date")
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	asset := []byte("tampered")
	srv := fakeGitHub(t, "v2.0.0", map[string][]byte{
		"argus-agent_linux_amd64": asset,
		"checksums.txt":           []byte(strings.Repeat("0", 64) + "  argus-agent_linux_amd64\n"),
	})
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "argus-agent")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := New("o/r", "argus-agent_linux_amd64", "v1.0.0")
	u.apiBase = srv.URL
	if _, _, err := u.Update(context.Background(), exe); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "old" {
		t.Fatal("file modified despite checksum mismatch")
	}
}

func TestUpdateMissingAsset(t *testing.T) {
	srv := fakeGitHub(t, "v2.0.0", map[string][]byte{
		"checksums.txt": []byte("deadbeef  other\n"),
	})
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "argus-agent")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := New("o/r", "argus-agent_linux_amd64", "v1.0.0")
	u.apiBase = srv.URL
	if _, _, err := u.Update(context.Background(), exe); err == nil {
		t.Fatal("expected missing-asset error")
	}
}
