package github

import (
	"fmt"
	"strconv"
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
	Token    string
	endpoint string
}

func NewGithub(token string) *Github {
	return &Github{Token: token, endpoint: "https://api.github.com/graphql"}
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
		queryType = fmt.Sprintf("organization(login: %s)", strconv.Quote(owner))
	} else {
		queryType = fmt.Sprintf("repositoryOwner(login: %s)", strconv.Quote(owner))
	}
	token := request.WithAuthorization(g.Token, "bearer")
	ownerLower := strings.ToLower(owner)
	for {
		query := map[string]string{"query": fmt.Sprintf(tmpl, queryType, next)}
		data, err := request.POST[reposQuery](g.endpoint, query, token)
		if err != nil {
			return nil, err
		}
		if len(data.Errors) > 0 {
			return nil, fmt.Errorf("GitHub GraphQL: %s", data.Errors[0].Message)
		}
		if data.Data.Repositories == nil {
			return nil, fmt.Errorf("GitHub GraphQL returned no repository owner for %q", owner)
		}
		if data.Data.Repositories.Repositories == nil || data.Data.Repositories.Repositories.PageInfo == nil || data.Data.Repositories.Repositories.Nodes == nil {
			return nil, fmt.Errorf("GitHub GraphQL returned an incomplete repository list for %q", owner)
		}
		for _, repo := range data.Data.Repositories.Repositories.Nodes {
			if strings.ToLower(repo.Owner.Login) == ownerLower {
				repos = append(repos, repo)
			}
		}
		if !data.Data.Repositories.Repositories.PageInfo.HasNextPage {
			break
		}
		if data.Data.Repositories.Repositories.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf("GitHub GraphQL returned an empty pagination cursor for %q", owner)
		}
		next = strconv.Quote(data.Data.Repositories.Repositories.PageInfo.EndCursor)
	}
	return repos, nil
}

type reposQuery struct {
	Data struct {
		Repositories *struct {
			Repositories *struct {
				PageInfo *struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []Repo `json:"nodes"`
			} `json:"repositories"`
		} `json:"repositories"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}
