package gitea

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("gitea backup config is missing")
	}
	endpoint, err := url.Parse(c.Host)
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("invalid Gitea host %q", c.Host)
	}
	return nil
}

var _ provider.Provider = &Gitea{}

type Gitea struct {
	conf *Config
}

func NewGitea(conf *Config) *Gitea {
	copy := *conf
	copy.Host = strings.TrimRight(copy.Host, "/")
	if !strings.HasSuffix(copy.Host, "/api/v1") {
		copy.Host += "/api/v1"
	}
	return &Gitea{conf: &copy}
}

func (g *Gitea) DestinationID() string {
	endpoint, err := url.Parse(g.conf.Host)
	if err != nil {
		return "gitea:" + g.conf.Host
	}
	endpoint.Scheme = strings.ToLower(endpoint.Scheme)
	endpoint.Host = strings.ToLower(endpoint.Host)
	return "gitea:" + endpoint.String()
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

func (g *Gitea) LoadRepos(ctx context.Context, owner *provider.Owner) ([]string, error) {
	limit := 100
	page := 1
	repos := make([]string, 0)
	ownerLower := strings.ToLower(owner.Name)
	for {
		url := fmt.Sprintf("%s/%s?limit=%d&page=%d", g.conf.Host, g.buildReposPath(owner.Name, owner.IsOrg), limit, page)
		res, err := request.GET[[]reposQuery](ctx, url, g.requestModifier()...)
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

func (g *Gitea) MigrateRepo(ctx context.Context, from *provider.Owner, to *provider.Owner, repo *provider.Repo) (string, error) {
	cloneAddr := fmt.Sprintf("https://github.com/%s/%s.git", from.Name, repo.Name)
	repoURL := fmt.Sprintf("%s/repos/%s/%s", g.conf.Host, to.Name, repo.Name)
	existing, err := request.GET[repoDetails](ctx, repoURL, g.requestModifier()...)
	if err == nil {
		if !existing.Mirror || !strings.EqualFold(strings.TrimSuffix(existing.OriginalURL, ".git"), strings.TrimSuffix(cloneAddr, ".git")) {
			return "", fmt.Errorf("destination %s/%s exists but is not a mirror of %s", to.Name, repo.Name, cloneAddr)
		}
		resp, err := request.Request(ctx, http.MethodPost, repoURL+"/mirror-sync", g.requestModifier()...)
		if err != nil {
			return "", fmt.Errorf("sync mirror %s/%s: %w", to.Name, repo.Name, err)
		}
		_ = resp.Body.Close()
		return "synced", nil
	}
	var statusErr *request.StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusNotFound {
		return "", fmt.Errorf("check destination %s/%s: %w", to.Name, repo.Name, err)
	}
	r := migrateRequest{
		RepoOwner:   to.Name,
		RepoName:    repo.Name,
		Description: repo.Description,
		Private:     true,

		AuthUsername: g.conf.AuthUsername,
		AuthToken:    repo.AuthToken,

		MirrorInterval: "10m0s",
		Service:        "github",
		CloneAddr:      cloneAddr,
		Mirror:         true,
	}
	url := fmt.Sprintf("%s/repos/migrate", g.conf.Host)
	res, err := request.POST[reposQuery](ctx, url, r, g.requestModifier()...)
	if err != nil {
		return "", err
	}
	return (*res).Name, nil
}

func (g *Gitea) DeleteRepo(ctx context.Context, owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", g.conf.Host, owner, repo)
	resp, err := request.Request(ctx, http.MethodDelete, url, g.requestModifier()...)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
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

type repoDetails struct {
	Name        string `json:"name"`
	Mirror      bool   `json:"mirror"`
	OriginalURL string `json:"original_url"`
}
