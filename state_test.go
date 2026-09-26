package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
