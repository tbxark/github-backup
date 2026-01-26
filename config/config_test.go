package config

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tbxark/github-backup/provider/gitea"
)

func TestSyncConfig(t *testing.T) {
	c := SyncConfig{
		DefaultConf: &DefaultConfig{
			GithubToken: "YOUR_GITHUB_TOKEN",
			RepoOwner:   "BACKUP_TARGET_REPO_OWNER",
			Backup: &BackupProviderConfig{
				Type: "gitea",
				Config: ToRaw(&gitea.Config{
					Host:         "GITEA_HOST",
					Token:        "GITEA_TOKEN",
					AuthUsername: "GITEA_USERNAME",
				}),
			},
			Filter: &FilterConfig{
				UnmatchedRepoAction: UnmatchedRepoActionDelete,
				AllowRule: []string{
					// :owner/:repo/:private/:fork/:archived
					"[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/1/[01]/[01]",
				},
				DenyRule: []string{
					// :owner/:repo/:private/:fork/:archived
					"[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/0/[01]/[01]",
				},
			},
			SpecificGithubToken: map[string]string{
				"[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/1/[01]/[01]": "PRIVATE_GITHUB_TOKEN",
				"[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/0/[01]/[01]": "PUBLIC_GITHUB_TOKEN",
			},
		},
		Targets: []*GithubConfig{
			{
				Owner:          "GITHUB_ORG",
				RepoOwner:      "BACKUP_TARGET_REPO_ORG",
				IsOwnerOrg:     true,
				IsRepoOwnerOrg: true,
			},
		},
	}
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(bytes))
	err = json.Unmarshal(bytes, &c)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%+v\n", c)
}
