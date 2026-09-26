package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"regexp"
	"slices"
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

type sourceLoader func(context.Context, string, string, bool) ([]github.Repo, error)

type SyncTask struct {
	conf          *config.SyncConfig
	counter       map[string]int
	mu            sync.Mutex
	loadSource    sourceLoader
	buildProvider func(*config.BackupProviderConfig) (provider.Provider, error)
	Interactive   bool
}

func NewTask(conf *config.SyncConfig) *SyncTask {
	return &SyncTask{
		conf:    conf,
		counter: make(map[string]int),
		loadSource: func(ctx context.Context, token, owner string, isOrg bool) ([]github.Repo, error) {
			return github.NewGithub(token).LoadAllRepos(ctx, owner, isOrg)
		},
		buildProvider: BuildBackupProvider,
		Interactive:   true,
	}
}

func (t *SyncTask) Run(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.loadCounters(); err != nil {
		return err
	}
	var errs []error
	groups := make(map[string]*destinationGroup)
	var runs []*targetRun
	preparationFailed := false
	for _, target := range t.conf.Targets {
		run, err := t.prepareTarget(target)
		if err != nil {
			errs = append(errs, err)
			preparationFailed = true
			continue
		}
		key := run.backup.DestinationID() + "\x00" + run.to.Name
		if target.Backup.Type == config.BackupProviderConfigTypeGitea {
			key = run.backup.DestinationID() + "\x00" + strings.ToLower(run.to.Name)
		}
		group := groups[key]
		if group == nil {
			group = &destinationGroup{
				key: key, backup: run.backup, owner: run.to, keep: make(map[string]struct{}),
				caseInsensitive: target.Backup.Type == config.BackupProviderConfigTypeGitea,
			}
			groups[key] = group
		} else if confirmer, ok := run.backup.(interface{ RequiresConfirmation() bool }); ok && confirmer.RequiresConfirmation() {
			group.backup = run.backup
		}
		group.targets = append(group.targets, run.target)
		group.action = mergeAction(group.action, run.target.Filter.UnmatchedRepoAction)
		group.checkCount = max(group.checkCount, run.target.Filter.PreDeleteCheckCount)
		run.group = group
		runs = append(runs, run)
	}
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := t.migrateTarget(ctx, run); err != nil {
			run.group.failed = true
			errs = append(errs, err)
		}
	}
	if !preparationFailed && ctx.Err() == nil {
		for _, key := range slices.Sorted(maps.Keys(groups)) {
			group := groups[key]
			if group.failed || group.action == config.UnmatchedRepoActionIgnore {
				continue
			}
			if err := t.deleteUnmatched(ctx, group); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if err := t.saveCounters(); err != nil {
		errs = append(errs, err)
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type tokenRule struct {
	pattern *regexp.Regexp
	token   string
}

type targetRun struct {
	target *config.GithubConfig
	backup provider.Provider
	from   *provider.Owner
	to     *provider.Owner
	allow  []*regexp.Regexp
	deny   []*regexp.Regexp
	tokens []tokenRule
	group  *destinationGroup
}

type destinationGroup struct {
	key             string
	backup          provider.Provider
	owner           *provider.Owner
	targets         []*config.GithubConfig
	keep            map[string]struct{}
	action          config.UnmatchedRepoAction
	checkCount      int
	failed          bool
	caseInsensitive bool
}

func (g *destinationGroup) repoKey(name string) string {
	if g.caseInsensitive {
		return strings.ToLower(name)
	}
	return name
}

func mergeAction(a, b config.UnmatchedRepoAction) config.UnmatchedRepoAction {
	if a == config.UnmatchedRepoActionAsk || b == config.UnmatchedRepoActionAsk {
		return config.UnmatchedRepoActionAsk
	}
	if a == config.UnmatchedRepoActionDelete || b == config.UnmatchedRepoActionDelete {
		return config.UnmatchedRepoActionDelete
	}
	return config.UnmatchedRepoActionIgnore
}

func compilePatterns(kind string, rules []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(rules))
	for _, rule := range rules {
		pattern, err := regexp.Compile(rule)
		if err != nil {
			return nil, fmt.Errorf("invalid %s rule %q: %w", kind, rule, err)
		}
		compiled = append(compiled, pattern)
	}
	return compiled, nil
}

func (t *SyncTask) prepareTarget(target *config.GithubConfig) (*targetRun, error) {
	if target == nil {
		return nil, errors.New("nil backup target")
	}
	target.MergeDefault(t.conf.DefaultConf)
	if target.Filter.PreDeleteCheckCount < 0 {
		return nil, fmt.Errorf("target %s has negative pre_delete_check_count", target.Owner)
	}
	if target.Filter.PreDeleteCheckCount > 0 && t.conf.StateFile == "" {
		return nil, fmt.Errorf("target %s requires state_file for pre_delete_check_count", target.Owner)
	}
	switch target.Filter.UnmatchedRepoAction {
	case config.UnmatchedRepoActionDelete, config.UnmatchedRepoActionAsk, config.UnmatchedRepoActionIgnore:
	default:
		return nil, fmt.Errorf("target %s has unknown unmatched_repo_action %q", target.Owner, target.Filter.UnmatchedRepoAction)
	}
	allow, err := compilePatterns("allow", target.Filter.AllowRule)
	if err != nil {
		return nil, fmt.Errorf("target %s: %w", target.Owner, err)
	}
	deny, err := compilePatterns("deny", target.Filter.DenyRule)
	if err != nil {
		return nil, fmt.Errorf("target %s: %w", target.Owner, err)
	}
	tokens := make([]tokenRule, 0, len(target.SpecificGithubToken))
	for _, rule := range slices.Sorted(maps.Keys(target.SpecificGithubToken)) {
		pattern, err := regexp.Compile(rule)
		if err != nil {
			return nil, fmt.Errorf("target %s: invalid token rule %q: %w", target.Owner, rule, err)
		}
		tokens = append(tokens, tokenRule{pattern: pattern, token: target.SpecificGithubToken[rule]})
	}
	backup, err := t.buildProvider(target.Backup)
	if err != nil {
		return nil, fmt.Errorf("build backup provider for %s: %w", target.Owner, err)
	}
	if interactive, ok := backup.(interface{ SetInteractive(bool) }); ok {
		interactive.SetInteractive(t.Interactive)
	}
	return &targetRun{
		target: target, backup: backup,
		from:  &provider.Owner{Name: target.Owner, IsOrg: target.IsOwnerOrg},
		to:    &provider.Owner{Name: target.RepoOwner, IsOrg: target.IsRepoOwnerOrg},
		allow: allow, deny: deny, tokens: tokens,
	}, nil
}

func matchesAny(identity string, rules []*regexp.Regexp) bool {
	for _, rule := range rules {
		if rule.MatchString(identity) {
			return true
		}
	}
	return false
}

func (r *targetRun) repoAllowed(identity string) bool {
	return (len(r.allow) == 0 || matchesAny(identity, r.allow)) && !matchesAny(identity, r.deny)
}

func (t *SyncTask) migrateTarget(ctx context.Context, run *targetRun) error {
	repos, err := t.loadSource(ctx, run.target.Token, run.target.Owner, run.target.IsOwnerOrg)
	if err != nil {
		return fmt.Errorf("load GitHub repos for %s: %w", run.target.Owner, err)
	}
	log.Printf("found %d repos in %s", len(repos), run.target.Owner)
	var errs []error
	for _, repo := range repos {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		identity := matcher.Identity(run.target.Owner, repo.Name, repo.Private, repo.Fork, repo.Archived)
		if !run.repoAllowed(identity) {
			if run.target.Filter.UnmatchedRepoAction == config.UnmatchedRepoActionIgnore {
				run.group.keep[run.group.repoKey(repo.Name)] = struct{}{}
			}
			continue
		}
		run.group.keep[run.group.repoKey(repo.Name)] = struct{}{}
		delete(t.counter, counterKey(run.target, repo.Name))
		delete(t.counter, run.group.counterKey(repo.Name))
		token := run.target.Token
		for _, rule := range run.tokens {
			if rule.pattern.MatchString(identity) {
				token = rule.token
				break
			}
		}
		status, err := run.backup.MigrateRepo(ctx, run.from, run.to, &provider.Repo{
			Name: repo.Name, Description: repo.Description, AuthToken: token,
		})
		if err != nil {
			log.Printf("migrate %s error: %s", repo.Name, err)
			errs = append(errs, fmt.Errorf("migrate %s/%s: %w", run.target.Owner, repo.Name, err))
		} else {
			log.Printf("migrate %s %s", repo.Name, status)
		}
	}
	return errors.Join(errs...)
}

func (t *SyncTask) deleteUnmatched(ctx context.Context, group *destinationGroup) error {
	repos, err := group.backup.LoadRepos(ctx, group.owner)
	if err != nil {
		return fmt.Errorf("load destination repos for %s: %w", group.owner.Name, err)
	}
	var toDelete []string
	for _, repo := range repos {
		if _, ok := group.keep[group.repoKey(repo)]; ok {
			continue
		}
		key := group.counterKey(repo)
		if t.counter[key] < group.checkCount {
			t.counter[key]++
			continue
		}
		toDelete = append(toDelete, repo)
	}
	if len(toDelete) == 0 {
		return nil
	}
	if group.action == config.UnmatchedRepoActionAsk {
		if !t.Interactive {
			log.Printf("ask mode: skip deleting %d unmatched repo(s) because not running in interactive mode", len(toDelete))
			return nil
		}
		fmt.Printf("\nThe following %d repo(s) in %s are unmatched and will be deleted:\n", len(toDelete), group.owner.Name)
		for _, repo := range toDelete {
			fmt.Printf("  - %s\n", repo)
		}
		fmt.Print("Are you sure you want to delete these repos? (yes/no): ")
		if !confirmDeletion(ctx) {
			log.Printf("ask mode: deletion cancelled by user")
			return nil
		}
	}
	var errs []error
	for _, repo := range toDelete {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		status, err := group.backup.DeleteRepo(ctx, group.owner.Name, repo)
		if err != nil {
			log.Printf("delete %s error: %s", repo, err)
			errs = append(errs, fmt.Errorf("delete %s/%s: %w", group.owner.Name, repo, err))
		} else if status != "skip" {
			log.Printf("delete %s %s", repo, status)
			delete(t.counter, group.counterKey(repo))
		}
	}
	return errors.Join(errs...)
}

func confirmDeletion(ctx context.Context) bool {
	answer := make(chan bool, 1)
	go func() {
		input, err := bufio.NewReader(os.Stdin).ReadString('\n')
		answer <- err == nil && strings.EqualFold(strings.TrimSpace(input), "yes")
	}()
	select {
	case result := <-answer:
		return result
	case <-ctx.Done():
		return false
	}
}
