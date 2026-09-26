package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tbxark/github-backup/config"
)

func TestDeletionCountsPersistAndSeparateTargets(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	conf := &config.SyncConfig{StateFile: stateFile}
	first := &config.GithubConfig{Owner: "source", RepoOwner: "dest-one", Backup: &config.BackupProviderConfig{Type: "local"}}
	second := &config.GithubConfig{Owner: "source", RepoOwner: "dest-two", Backup: &config.BackupProviderConfig{Type: "local"}}
	firstKey := counterKey(first, "same-name")
	secondKey := counterKey(second, "same-name")
	if firstKey == secondKey {
		t.Fatal("different destinations share a deletion counter")
	}
	original := NewTask(conf)
	original.counter[firstKey] = 2
	if err := original.saveCounters(); err != nil {
		t.Fatal(err)
	}
	restarted := NewTask(conf)
	if err := restarted.loadCounters(); err != nil {
		t.Fatal(err)
	}
	if restarted.counter[firstKey] != 2 || restarted.counter[secondKey] != 0 {
		t.Fatalf("unexpected counters after restart: %+v", restarted.counter)
	}
}

func TestCorruptDeletionStateFailsClosed(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(stateFile, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	task := NewTask(&config.SyncConfig{StateFile: stateFile})
	if err := task.Run(context.Background()); err == nil {
		t.Fatal("corrupt state was ignored")
	}
}

func TestStateLockRespectsContext(t *testing.T) {
	conf := &config.SyncConfig{StateFile: filepath.Join(t.TempDir(), "state.json")}
	first := NewTask(conf)
	unlock, err := first.lockState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	second := NewTask(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := second.lockState(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock error = %v, want deadline", err)
	}
}

func TestDeletionCountersPruneReposMissingFromDestination(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 2))
	backup.repos = []string{"one"}
	key := counterKey(task.conf.Targets[0], "previously-deleted")
	task.counter[key] = 2
	if err := task.saveCounters(); err != nil {
		t.Fatal(err)
	}
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.counter[key]; ok {
		t.Fatal("stale counter was retained")
	}
}
