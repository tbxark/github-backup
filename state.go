package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/tbxark/github-backup/config"
)

func counterKey(target *config.GithubConfig, repo string) string {
	identity, _ := json.Marshal(struct {
		Source string
		Owner  string
		IsOrg  bool
		Backup *config.BackupProviderConfig
	}{target.Owner, target.RepoOwner, target.IsRepoOwnerOrg, target.Backup})
	hash := sha256.Sum256(identity)
	return fmt.Sprintf("%x/%s", hash, repo)
}

func (g *destinationGroup) counterKey(repo string) string {
	return g.counterPrefix() + repo
}

func (g *destinationGroup) counterPrefix() string {
	if len(g.targets) == 1 {
		return counterKey(g.targets[0], "")
	}
	hash := sha256.Sum256([]byte(g.key))
	return fmt.Sprintf("destination:%x/", hash)
}

func sourceCountKey(target *config.GithubConfig) string {
	identity, _ := json.Marshal(struct {
		Owner string
		IsOrg bool
	}{strings.ToLower(target.Owner), target.IsOwnerOrg})
	hash := sha256.Sum256(identity)
	return fmt.Sprintf("source:%x", hash)
}

func (t *SyncTask) lockState(ctx context.Context) (func() error, error) {
	if t.conf.StateFile == "" {
		return func() error { return nil }, nil
	}
	if err := os.MkdirAll(filepath.Dir(t.conf.StateFile), 0700); err != nil {
		return nil, fmt.Errorf("create deletion state directory: %w", err)
	}
	lock := flock.New(t.conf.StateFile + ".lock")
	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("lock deletion state: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("lock deletion state: lock was not acquired")
	}
	return lock.Unlock, nil
}

func (t *SyncTask) loadCounters() error {
	if t.conf.StateFile == "" {
		return nil
	}
	data, err := os.ReadFile(t.conf.StateFile)
	if os.IsNotExist(err) {
		t.counter = make(map[string]int)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read deletion state: %w", err)
	}
	var counters map[string]int
	if err := json.Unmarshal(data, &counters); err != nil {
		return fmt.Errorf("parse deletion state: %w", err)
	}
	for key, count := range counters {
		if count < 0 {
			return fmt.Errorf("invalid deletion count for %q", key)
		}
	}
	if counters == nil {
		counters = make(map[string]int)
	}
	t.counter = counters
	return nil
}

func (t *SyncTask) saveCounters() error {
	if t.conf.StateFile == "" {
		return nil
	}
	parent := filepath.Dir(t.conf.StateFile)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fmt.Errorf("create deletion state directory: %w", err)
	}
	tmp, err := os.CreateTemp(parent, ".github-backup-state-*")
	if err != nil {
		return fmt.Errorf("create temporary deletion state: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := json.NewEncoder(tmp).Encode(t.counter); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write deletion state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync deletion state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close deletion state: %w", err)
	}
	if err := os.Rename(tmp.Name(), t.conf.StateFile); err != nil {
		return fmt.Errorf("replace deletion state: %w", err)
	}
	return nil
}
