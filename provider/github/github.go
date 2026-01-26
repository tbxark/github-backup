package github

import (
	"fmt"
	"strings"

	"github.com/tbxark/github-backup/utils/request"
)

type Repo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"isPrivate"`
	Fork        bool   `json:"isFork"`
	Archived    bool   `json:"isArchived"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type Github struct {
	Token string
}

func NewGithub(token string) *Github {
	return &Github{Token: token}
}

func (g *Github) LoadAllRepos(owner string, isOrg bool) ([]Repo, error) {
	tmpl := `
query {
  repositories: %s {
    repositories(
      first: 100,
      after: %s
    ) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        name
        description
        isPrivate
        isFork
	    isArchived
        owner {
          login
        }
      }
    }
  }
}
`
	next := "null"
	queryType := ""
	var repos []Repo
	if isOrg {
		queryType = fmt.Sprintf("organization(login: \"%s\")", owner)
	} else {
		queryType = fmt.Sprintf("repositoryOwner(login: \"%s\")", owner)
	}
	token := request.WithAuthorization(g.Token, "bearer")
	ownerLower := strings.ToLower(owner)
	for {
		query := map[string]string{"query": fmt.Sprintf(tmpl, queryType, next)}
		data, err := request.POST[reposQuery]("https://api.github.com/graphql", query, token)
		if err != nil {
			return nil, err
		}
		for _, repo := range data.Data.Repositories.Repositories.Nodes {
			if strings.ToLower(repo.Owner.Login) == ownerLower {
				repos = append(repos, repo)
			}
		}
		if !data.Data.Repositories.Repositories.PageInfo.HasNextPage {
			break
		}
		next = fmt.Sprintf(`"%s"`, data.Data.Repositories.Repositories.PageInfo.EndCursor)
	}
	return repos, nil
}

type reposQuery struct {
	Data struct {
		Repositories struct {
			Repositories struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []Repo `json:"nodes"`
			} `json:"repositories"`
		} `json:"repositories"`
	} `json:"data"`
}
