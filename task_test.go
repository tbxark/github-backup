package main

import (
	"testing"

	"github.com/tbxark/github-backup/config"
)

func TestRepoAllowed(t *testing.T) {
	identity := "owner/repo/1/0/0"
	for _, tc := range []struct {
		name   string
		filter *config.FilterConfig
		want   bool
	}{
		{"no rules", &config.FilterConfig{}, true},
		{"allow miss", &config.FilterConfig{AllowRule: []string{`/0/0/0$`}}, false},
		{"allow match", &config.FilterConfig{AllowRule: []string{`/1/0/0$`}}, true},
		{"deny match", &config.FilterConfig{DenyRule: []string{`/1/0/0$`}}, false},
		{"deny overrides allow", &config.FilterConfig{AllowRule: []string{`/1/0/0$`}, DenyRule: []string{`/1/0/0$`}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoAllowed(identity, tc.filter); got != tc.want {
				t.Fatalf("repoAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}
