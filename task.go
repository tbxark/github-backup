package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

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
	conf        *config.SyncConfig
	counter     map[string]int
	Interactive bool
}

func NewTask(conf *config.SyncConfig) *SyncTask {
	return &SyncTask{
		conf:        conf,
		counter:     make(map[string]int, 100),
		Interactive: true,
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
	if target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionDelete ||
		target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionAsk {
		// load local repos
		localRepos, lErr := backup.LoadRepos(to)
		if lErr != nil {
			log.Panicf("load %s repos error: %s", target.RepoOwner, lErr.Error())
		}

		// collect repos to delete
		var toDelete []string
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
			toDelete = append(toDelete, repo)
		}

		if len(toDelete) == 0 {
			return
		}

		// ask mode requires interactive confirmation
		if target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionAsk {
			if !t.Interactive {
				log.Printf("ask mode: skip deleting %d unmatched repo(s) because not running in interactive mode", len(toDelete))
				return
			}
			fmt.Printf("\nThe following %d repo(s) in %s are unmatched and will be deleted:\n", len(toDelete), target.RepoOwner)
			for _, repo := range toDelete {
				fmt.Printf("  - %s\n", repo)
			}
			fmt.Print("Are you sure you want to delete these repos? (yes/no): ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				log.Printf("ask mode: read confirmation error: %s, skip deleting", err.Error())
				return
			}
			if strings.TrimSpace(strings.ToLower(input)) != "yes" {
				log.Printf("ask mode: deletion cancelled by user")
				return
			}
		}

		// delete collected repos
		for _, repo := range toDelete {
			s, e := backup.DeleteRepo(target.RepoOwner, repo)
			if e != nil {
				log.Printf("delete %s error: %s", repo, e.Error())
			} else {
				log.Printf("delete %s %s", repo, s)
			}
		}
	}
}
