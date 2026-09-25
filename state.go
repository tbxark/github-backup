package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
