package config

import (
	"os"
	"path/filepath"
	"testing"
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
	conf, err := NewConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.StateFile != statePath {
		t.Fatalf("state path = %q, want %q", conf.StateFile, statePath)
	}
}
