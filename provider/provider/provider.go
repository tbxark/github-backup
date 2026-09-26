package provider

import "context"

type Owner struct {
	Name  string
	IsOrg bool
}

type Repo struct {
	Name        string
	Description string
	AuthToken   string
}

type Provider interface {
	DestinationID() string
	LoadRepos(ctx context.Context, owner *Owner) ([]string, error)
	MigrateRepo(ctx context.Context, from *Owner, to *Owner, repo *Repo) (string, error)
	DeleteRepo(ctx context.Context, owner, repo string) (string, error)
}
