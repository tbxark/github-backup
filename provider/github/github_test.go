package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadAllReposRejectsAPIError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"Bad credentials"}`},
		{"graphql error", http.StatusOK, `{"data":null,"errors":[{"message":"Resource not accessible"}]}`},
		{"missing owner", http.StatusOK, `{"data":{"repositories":null}}`},
		{"missing list", http.StatusOK, `{"data":{"repositories":{"repositories":null}}}`},
		{"missing nodes", http.StatusOK, `{"data":{"repositories":{"repositories":{"pageInfo":{"hasNextPage":false},"nodes":null}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewGithub("token")
			client.endpoint = server.URL
			if _, err := client.LoadAllRepos(context.Background(), "owner", false); err == nil {
				t.Fatal("expected API error, got an empty repository list")
			}
		})
	}
}

func TestLoadAllReposPaginatesAndFiltersOwner(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":{"repositories":{"repositories":{"pageInfo":{"hasNextPage":true,"endCursor":"cursor"},"nodes":[{"name":"one","owner":{"login":"OWNER"}},{"name":"other","owner":{"login":"elsewhere"}}]}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"repositories":{"repositories":{"pageInfo":{"hasNextPage":false},"nodes":[{"name":"two","owner":{"login":"owner"}}]}}}}`))
	}))
	defer server.Close()
	client := NewGithub("token")
	client.endpoint = server.URL
	repos, err := client.LoadAllRepos(context.Background(), "owner", false)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(repos) != 2 || repos[0].Name != "one" || repos[1].Name != "two" {
		t.Fatalf("calls=%d repos=%+v", calls, repos)
	}
}

func TestLoadAllReposRejectsMissingCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repositories":{"repositories":{"pageInfo":{"hasNextPage":true,"endCursor":""},"nodes":[]}}}}`))
	}))
	defer server.Close()
	client := NewGithub("token")
	client.endpoint = server.URL
	_, err := client.LoadAllRepos(context.Background(), "owner", false)
	if err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("expected cursor error, got %v", err)
	}
}
