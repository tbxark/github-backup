package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeDefaultWithoutFilter(t *testing.T) {
	for _, defaults := range []*DefaultConfig{nil, {}} {
		c := &GithubConfig{Owner: "source"}
		c.MergeDefault(defaults)
		if c.Filter == nil || c.Filter.UnmatchedRepoAction != UnmatchedRepoActionIgnore || c.RepoOwner != "source" {
			t.Fatalf("unexpected merged config: %+v", c)
		}
	}
}

func TestMergeDefaultDoesNotShareFilter(t *testing.T) {
	defaults := &DefaultConfig{Filter: &FilterConfig{AllowRule: []string{"one"}}}
	first := &GithubConfig{Owner: "first"}
	second := &GithubConfig{Owner: "second"}
	first.MergeDefault(defaults)
	second.MergeDefault(defaults)
	first.Filter.AllowRule[0] = "changed"
	if second.Filter.AllowRule[0] != "one" || defaults.Filter.AllowRule[0] != "one" {
		t.Fatal("default filter was shared between targets")
	}
}

func TestConvertRejectsNull(t *testing.T) {
	if _, err := Convert[struct{}]([]byte("null")); err == nil {
		t.Fatal("null provider config was accepted")
	}
}

func TestNewConfigStateFileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("GITHUB_BACKUP_STATE_FILE", statePath)
	conf, err := NewConfig(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.StateFile != statePath {
		t.Fatalf("state path = %q, want %q", conf.StateFile, statePath)
	}
}

func TestNewConfigUsesCallerContext(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := NewConfig(ctx, server.URL)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("config request did not reach server")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("config error = %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("config load did not stop after cancellation")
	}
}
