package main

import (
	"fmt"
	"log"

	"github.com/tbxark/github-backup/config"
	"github.com/tbxark/github-backup/provider/gitea"
	"github.com/tbxark/github-backup/provider/github"
	"github.com/tbxark/github-backup/provider/local"
	"github.com/tbxark/github-backup/provider/provider"
	"github.com/tbxark/github-backup/utils/matcher"
)

func BuildBackupProvider(conf *config.BackupProviderConfig) (provider.Provider, error) {
	switch conf.Type {
	case config.BackupProviderConfigTypeGitea:
		c, err := config.Convert[gitea.Config](conf.Config)
		if err != nil {
			return nil, err
		}
		return gitea.NewGitea(c), nil
	case config.BackupProviderConfigTypeLocal:
		c, err := config.Convert[local.Config](conf.Config)
		if err != nil {
			return nil, err
		}
		return local.NewLocal(c), nil
	}
	return nil, fmt.Errorf("unknown backup provider type: %s", conf.Type)
}

type SyncTask struct {
	conf    *config.SyncConfig
	counter map[string]int
}

func NewTask(conf *config.SyncConfig) *SyncTask {
	return &SyncTask{
		conf:    conf,
		counter: make(map[string]int, 100),
	}
}

func (t *SyncTask) Run() {
	for _, target := range t.conf.Targets {
		t.execute(target)
	}
}

func (t *SyncTask) execute(target *config.GithubConfig) {
	// merge default config
	target.MergeDefault(t.conf.DefaultConf)

	// load all github repos
	loader := github.NewGithub(target.Token)
	repos, err := loader.LoadAllRepos(target.Owner, target.IsOwnerOrg)
	if err != nil {
		log.Panicf("load %s repos error: %s", target.RepoOwner, err.Error())
	}

	// build backup provider
	backup, err := BuildBackupProvider(target.Backup)
	if err != nil {
		log.Panicf("build backup provider error: %s", err.Error())
	}

	// handle repos set
	handledRepos := make(map[string]struct{})

	from := &provider.Owner{
		Name:  target.Owner,
		IsOrg: target.IsOwnerOrg,
	}
	to := &provider.Owner{
		Name:  target.RepoOwner,
		IsOrg: target.IsRepoOwnerOrg,
	}

	log.Printf("found %d repos in %s", len(repos), target.Owner)
	for _, repo := range repos {
		// render repo identity
		identity := matcher.Identity(target.Owner, repo.Name, repo.Private, repo.Fork, repo.Archived)

		// check allow/deny rule
		if target.Filter != nil {
			if !matcher.IsMatch(identity, target.Filter.AllowRule...) {
				if matcher.IsMatch(identity, target.Filter.DenyRule...) {
					continue
				}
			}
		}

		githubToken := target.Token
		// check specific GitHub token for this repo by regex
		for k, v := range target.SpecificGithubToken {
			if matcher.IsMatch(identity, k) {
				githubToken = v
				break
			}
		}

		// migrate repo
		delete(t.counter, repo.Name)

		s, e := backup.MigrateRepo(from, to, &provider.Repo{
			Name:        repo.Name,
			Description: repo.Description,
			AuthToken:   githubToken,
		})
		if e != nil {
			log.Printf("migrate %s error: %s", repo.Name, e.Error())
		} else {
			log.Printf("migrate %s %s", repo.Name, s)
		}
		handledRepos[repo.Name] = struct{}{}
	}

	// delete unmatched repos if needed
	if target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionDelete {
		// load local repos
		localRepos, lErr := backup.LoadRepos(to)
		if lErr != nil {
			log.Panicf("load %s repos error: %s", target.RepoOwner, lErr.Error())
		}
		// delete unmatched repos
		for _, repo := range localRepos {
			if _, ok := handledRepos[repo]; ok {
				continue
			}
			if target.Filter.PreDeleteCheckCount > 0 {
				if t.counter[repo] < target.Filter.PreDeleteCheckCount {
					t.counter[repo]++
					continue
				}
			}
			s, e := backup.DeleteRepo(target.RepoOwner, repo)
			if e != nil {
				log.Printf("delete %s error: %s", repo, e.Error())
			} else {
				log.Printf("delete %s %s", repo, s)
			}
		}
	}
}
