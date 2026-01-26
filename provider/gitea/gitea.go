package gitea

import (
	"fmt"
	"strings"

	"github.com/tbxark/github-backup/provider/provider"
	"github.com/tbxark/github-backup/utils/request"
)

type Config struct {
	Host         string `json:"host"`
	Token        string `json:"token"`
	AuthToken    string `json:"auth_token"`
	AuthUsername string `json:"auth_username"`
}

var _ provider.Provider = &Gitea{}

type Gitea struct {
	conf *Config
}

func NewGitea(conf *Config) *Gitea {
	conf.Host = strings.TrimRight(conf.Host, "/")
	if !strings.HasSuffix(conf.Host, "/api/v1") {
		conf.Host += "/api/v1"
	}
	return &Gitea{conf: conf}
}

func (g *Gitea) buildReposPath(owner string, isOrg bool) string {
	if isOrg {
		return fmt.Sprintf("orgs/%s/repos", owner)
	} else {
		return fmt.Sprintf("users/%s/repos", owner)
	}
}

func (g *Gitea) requestModifier() []request.Modifier {
	return []request.Modifier{
		request.WithAuthorization(g.conf.Token, "token"),
	}
}

func (g *Gitea) LoadRepos(owner *provider.Owner) ([]string, error) {
	limit := 100
	page := 1
	repos := make([]string, 0)
	ownerLower := strings.ToLower(owner.Name)
	for {
		url := fmt.Sprintf("%s/%s?limit=%d&page=%d", g.conf.Host, g.buildReposPath(owner.Name, owner.IsOrg), limit, page)
		res, err := request.GET[[]reposQuery](url, g.requestModifier()...)
		if err != nil {
			return nil, err
		}
		for _, r := range *res {
			if strings.ToLower(r.Owner.Login) == ownerLower {
				repos = append(repos, r.Name)
			}
		}
		if len(*res) < limit {
			break
		}
		page += 1
	}
	return repos, nil
}

func (g *Gitea) MigrateRepo(from *provider.Owner, to *provider.Owner, repo *provider.Repo) (string, error) {
	r := migrateRequest{
		RepoOwner:   to.Name,
		RepoName:    repo.Name,
		Description: repo.Description,
		Private:     true,

		AuthUsername: g.conf.AuthUsername,
		AuthToken:    repo.AuthToken,

		MirrorInterval: "10m0s",
		Service:        "github",
		CloneAddr:      fmt.Sprintf("https://github.com/%s/%s.git", from.Name, repo.Name),
		Mirror:         true,
	}
	url := fmt.Sprintf("%s/repos/migrate", g.conf.Host)
	res, err := request.POST[reposQuery](url, r, g.requestModifier()...)
	if err != nil {
		return "", err
	}
	return (*res).Name, nil
}

func (g *Gitea) DeleteRepo(owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", g.conf.Host, owner, repo)
	resp, err := request.Request("DELETE", url, g.requestModifier()...)
	if err != nil {
		return "", err
	}
	return resp.Status, nil
}

type migrateRequest struct {
	RepoName    string `json:"repo_name"`
	RepoOwner   string `json:"repo_owner"`
	Description string `json:"description"`
	Private     bool   `json:"private"`

	AuthUsername string `json:"auth_username"`
	AuthToken    string `json:"auth_token"`

	MirrorInterval string `json:"mirror_interval"`
	Service        string `json:"service"`
	CloneAddr      string `json:"clone_addr"`
	Mirror         bool   `json:"mirror"`
}

type reposQuery struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}
