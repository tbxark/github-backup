package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tbxark/github-backup/provider/provider"
)

func TestMigrateAndDeleteRejectHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
		} else if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
		_, _ = w.Write([]byte(`{"message":"request failed"}`))
	}))
	defer server.Close()
	client := NewGitea(&Config{Host: server.URL, Token: "token"})
	if _, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"}); err == nil {
		t.Fatal("migration reported success for HTTP 409")
	}
	if _, err := client.DeleteRepo(context.Background(), "dest", "repo"); err == nil {
		t.Fatal("deletion reported success for HTTP 403")
	}
}

func TestExistingMirrorIsSynced(t *testing.T) {
	syncCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/dest/repo":
			_, _ = w.Write([]byte(`{"name":"repo","mirror":true,"original_url":"https://github.com/source/repo.git"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/dest/repo/mirror-sync":
			syncCalls++
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/migrate":
			t.Error("existing mirror was migrated again")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewGitea(&Config{Host: server.URL, Token: "token"})
	result, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"})
	if err != nil || result != "synced" || syncCalls != 1 {
		t.Fatalf("result=%q err=%v syncCalls=%d", result, err, syncCalls)
	}
}

func TestExistingUnrelatedRepoIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"repo","mirror":false,"original_url":""}`))
	}))
	defer server.Close()
	client := NewGitea(&Config{Host: server.URL, Token: "token"})
	if _, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"}); err == nil {
		t.Fatal("unrelated destination repository was accepted")
	}
}

func TestDestinationIDPreservesPathCase(t *testing.T) {
	first := NewGitea(&Config{Host: "https://EXAMPLE.com/Team"})
	second := NewGitea(&Config{Host: "https://example.com/team"})
	if first.DestinationID() == second.DestinationID() {
		t.Fatal("distinct case-sensitive API paths share a destination")
	}
}
