// Package state manages the small metadata files under
// /var/lib/nanok8s/state/ that describe the most recent successful boot.
//
// Two files matter:
//
//   - last-boot.json: JSON metadata for the boot that last completed
//     successfully. Holds the nanok8s version, the ostree/bootc
//     deployment id (when applicable) and the kernel boot id. Used at
//     the start of the next boot to detect upgrades and to name the
//     backup of the data produced by that previous boot.
//   - last-event: human-readable one-liner describing the most recent
//     lifecycle event. Surfaced via greenboot wanted.d to MOTD.
//
// Rollback is triggered by an external marker file placed by the
// greenboot red.d hook; that logic lives in the backup package. No
// state file tracks rollback intent.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MatchaScript/nanok8s/internal/paths"
)

// LastBoot is the metadata persisted after a healthy boot. DeploymentID
// is empty on non-ostree systems where no bootc deployment exists.
type LastBoot struct {
	Version      string `json:"version"`
	DeploymentID string `json:"deploymentId,omitempty"`
	BootID       string `json:"bootId,omitempty"`
}

// ReadLastBoot returns the persisted metadata. The bool is false when no
// last-boot record exists (fresh install or post-reset).
func ReadLastBoot() (LastBoot, bool, error) {
	b, err := os.ReadFile(paths.LastBootFile)
	if errors.Is(err, os.ErrNotExist) {
		return LastBoot{}, false, nil
	}
	if err != nil {
		return LastBoot{}, false, fmt.Errorf("read last-boot: %w", err)
	}
	var lb LastBoot
	if err := json.Unmarshal(b, &lb); err != nil {
		return LastBoot{}, false, fmt.Errorf("parse last-boot: %w", err)
	}
	return lb, true, nil
}

// WriteLastBoot records lb atomically.
func WriteLastBoot(lb LastBoot) error {
	data, err := json.Marshal(lb)
	if err != nil {
		return err
	}
	return writeAtomic(paths.LastBootFile, data)
}

// WriteLastEvent records msg as the most recent lifecycle event.
func WriteLastEvent(msg string) error {
	return writeAtomic(paths.LastEventFile, []byte(msg+"\n"))
}

// ReadLastEvent returns the recorded event, or "" if none exists.
func ReadLastEvent() (string, error) {
	b, err := os.ReadFile(paths.LastEventFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Exists reports whether a prior `nanok8s bootstrap` has already run on
// this node. Returns true if any of:
//
//   - /var/lib/nanok8s/state/last-boot.json (a healthy boot completed)
//   - /var/lib/nanok8s/state/last-event (bootstrap or boot logged an event)
//   - /etc/kubernetes/manifests/kube-apiserver.yaml (bootstrap wrote the
//     static pod manifests even if nothing else touched state yet)
//
// The manifest check mirrors kubeadm init's preflight
// DirAvailable--etc-kubernetes-manifests gate: a populated manifests
// directory is sufficient evidence that an init has already happened.
func Exists() (bool, error) {
	for _, p := range []string{
		paths.LastBootFile,
		paths.LastEventFile,
		paths.ManifestsDir + "/kube-apiserver.yaml",
	} {
		ok, err := fileExists(p)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func fileExists(p string) (bool, error) {
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// writeAtomic writes data to path via a sibling temp file + rename so
// readers never see a half-written file.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
