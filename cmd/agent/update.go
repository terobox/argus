package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/terobox/argus/pkg/selfupdate"
)

// selfupdateRepo is the GitHub repository the release assets are published to.
const selfupdateRepo = "terobox/argus"

// assetName returns the release asset name goreleaser produces for this build
// (see the archives name_template in .goreleaser.yaml).
func assetName() string {
	return fmt.Sprintf("argus-agent_%s_%s", runtime.GOOS, runtime.GOARCH)
}

// runUpdate downloads and atomically installs the latest release over the
// running binary. The running process (if any) keeps executing the old inode;
// systemd's Restart=always restarts it into the new version, or the operator
// restarts it manually.
func runUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self-update: locate executable: %w", err)
	}

	u := selfupdate.New(selfupdateRepo, assetName(), agentVersion)
	version, applied, err := u.Update(context.Background(), exe)
	if err != nil {
		return err
	}
	if !applied {
		fmt.Printf("already up to date (%s)\n", version)
		return nil
	}
	fmt.Printf("updated to %s; restart to take effect\n", version)
	return nil
}
