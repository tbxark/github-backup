package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/tbxark/github-backup/config"
	"github.com/tbxark/github-backup/provider/gitea"
	"github.com/tbxark/github-backup/provider/github"
	"github.com/tbxark/github-backup/provider/local"
	"github.com/tbxark/github-backup/provider/provider"
	"github.com/tbxark/github-backup/utils/matcher"
)

func BuildBackupProvider(conf *config.BackupProviderConfig) (provider.Provider, error) {
	if conf == nil {
		return nil, errors.New("backup provider is not configured")
	}
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
	mu          sync.Mutex
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
	if err := t.RunOnce(); err != nil {
		log.Printf("backup failed: %s", err)
	}
}

func (t *SyncTask) RunOnce() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.loadCounters(); err != nil {
		return err
	}
	var errs []error
	for _, target := range t.conf.Targets {
		if err := t.execute(target); err != nil {
			errs = append(errs, err)
		}
	}
	if err := t.saveCounters(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (t *SyncTask) execute(target *config.GithubConfig) error {
	if target == nil {
		return errors.New("nil backup target")
	}
	// merge default config
	target.MergeDefault(t.conf.DefaultConf)
	if target.Filter.PreDeleteCheckCount > 0 && t.conf.StateFile == "" {
		return fmt.Errorf("target %s requires state_file for pre_delete_check_count", target.Owner)
	}

	// load all github repos
	loader := github.NewGithub(target.Token)
	repos, err := loader.LoadAllRepos(target.Owner, target.IsOwnerOrg)
	if err != nil {
		return fmt.Errorf("load GitHub repos for %s: %w", target.Owner, err)
	}

	// build backup provider
	backup, err := BuildBackupProvider(target.Backup)
	if err != nil {
		return fmt.Errorf("build backup provider for %s: %w", target.Owner, err)
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
	var errs []error
	for _, repo := range repos {
		// render repo identity
		identity := matcher.Identity(target.Owner, repo.Name, repo.Private, repo.Fork, repo.Archived)

		// check allow/deny rule
		if !repoAllowed(identity, target.Filter) {
			continue
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
		delete(t.counter, counterKey(target, repo.Name))

		s, e := backup.MigrateRepo(from, to, &provider.Repo{
			Name:        repo.Name,
			Description: repo.Description,
			AuthToken:   githubToken,
		})
		if e != nil {
			log.Printf("migrate %s error: %s", repo.Name, e.Error())
			errs = append(errs, fmt.Errorf("migrate %s/%s: %w", target.Owner, repo.Name, e))
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
			return errors.Join(append(errs, fmt.Errorf("load destination repos for %s: %w", target.RepoOwner, lErr))...)
		}

		// collect repos to delete
		var toDelete []string
		for _, repo := range localRepos {
			key := counterKey(target, repo)
			if _, ok := handledRepos[repo]; ok {
				continue
			}
			if target.Filter.PreDeleteCheckCount > 0 {
				if t.counter[key] < target.Filter.PreDeleteCheckCount {
					t.counter[key]++
					continue
				}
			}
			toDelete = append(toDelete, repo)
		}

		if len(toDelete) == 0 {
			return errors.Join(errs...)
		}

		// ask mode requires interactive confirmation
		if target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionAsk {
			if !t.Interactive {
				log.Printf("ask mode: skip deleting %d unmatched repo(s) because not running in interactive mode", len(toDelete))
				return errors.Join(errs...)
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
				return errors.Join(errs...)
			}
			if strings.TrimSpace(strings.ToLower(input)) != "yes" {
				log.Printf("ask mode: deletion cancelled by user")
				return errors.Join(errs...)
			}
		}

		// delete collected repos
		for _, repo := range toDelete {
			s, e := backup.DeleteRepo(target.RepoOwner, repo)
			if e != nil {
				log.Printf("delete %s error: %s", repo, e.Error())
				errs = append(errs, fmt.Errorf("delete %s/%s: %w", target.RepoOwner, repo, e))
			} else {
				log.Printf("delete %s %s", repo, s)
				delete(t.counter, counterKey(target, repo))
			}
		}
	}
	return errors.Join(errs...)
}

func repoAllowed(identity string, filter *config.FilterConfig) bool {
	if filter == nil {
		return true
	}
	if len(filter.AllowRule) > 0 && !matcher.IsMatch(identity, filter.AllowRule...) {
		return false
	}
	return !matcher.IsMatch(identity, filter.DenyRule...)
}
