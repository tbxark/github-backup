package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tbxark/github-backup/config"
	"github.com/tbxark/github-backup/provider/github"
	"github.com/tbxark/github-backup/provider/provider"
)

func TestRepoAllowed(t *testing.T) {
	identity := "owner/repo/1/0/0"
	for _, tc := range []struct {
		name   string
		filter *config.FilterConfig
		want   bool
	}{
		{"no rules", &config.FilterConfig{}, true},
		{"allow miss", &config.FilterConfig{AllowRule: []string{`/0/0/0$`}}, false},
		{"allow match", &config.FilterConfig{AllowRule: []string{`/1/0/0$`}}, true},
		{"deny match", &config.FilterConfig{DenyRule: []string{`/1/0/0$`}}, false},
		{"deny overrides allow", &config.FilterConfig{AllowRule: []string{`/1/0/0$`}, DenyRule: []string{`/1/0/0$`}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allow, err := compilePatterns("allow", tc.filter.AllowRule)
			if err != nil {
				t.Fatal(err)
			}
			deny, err := compilePatterns("deny", tc.filter.DenyRule)
			if err != nil {
				t.Fatal(err)
			}
			run := &targetRun{allow: allow, deny: deny}
			if got := run.repoAllowed(identity); got != tc.want {
				t.Fatalf("repoAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeBackup struct {
	repos      []string
	deleted    []string
	migrated   map[string]string
	migrateErr map[string]error
	migrateCtx context.Context
}

func (f *fakeBackup) DestinationID() string { return "fake:destination" }
func (f *fakeBackup) LoadRepos(context.Context, *provider.Owner) ([]string, error) {
	return slices.Clone(f.repos), nil
}
func (f *fakeBackup) MigrateRepo(ctx context.Context, from, _ *provider.Owner, repo *provider.Repo) (string, error) {
	f.migrateCtx = ctx
	if err := f.migrateErr[from.Name+"/"+repo.Name]; err != nil {
		return "", err
	}
	if f.migrated == nil {
		f.migrated = make(map[string]string)
	}
	f.migrated[from.Name+"/"+repo.Name] = repo.AuthToken
	if !slices.Contains(f.repos, repo.Name) {
		f.repos = append(f.repos, repo.Name)
	}
	return "success", nil
}
func (f *fakeBackup) DeleteRepo(_ context.Context, _, repo string) (string, error) {
	f.deleted = append(f.deleted, repo)
	f.repos = slices.DeleteFunc(f.repos, func(name string) bool { return name == repo })
	return "success", nil
}

func testTarget(owner string, checks int) *config.GithubConfig {
	return &config.GithubConfig{
		Owner: owner, RepoOwner: "dest",
		Backup: &config.BackupProviderConfig{Type: config.BackupProviderConfigTypeLocal},
		Filter: &config.FilterConfig{UnmatchedRepoAction: config.UnmatchedRepoActionDelete, PreDeleteCheckCount: checks},
	}
}

func testTask(t *testing.T, targets ...*config.GithubConfig) (*SyncTask, *fakeBackup) {
	t.Helper()
	backup := &fakeBackup{repos: []string{"one", "two", "stale"}}
	task := NewTask(&config.SyncConfig{Targets: targets, StateFile: filepath.Join(t.TempDir(), "state.json")})
	task.buildProvider = func(*config.BackupProviderConfig) (provider.Provider, error) { return backup, nil }
	task.loadSource = func(_ context.Context, _, owner string, _ bool) ([]github.Repo, error) {
		switch owner {
		case "first":
			return []github.Repo{{Name: "one"}}, nil
		case "second":
			return []github.Repo{{Name: "two"}}, nil
		}
		return nil, fmt.Errorf("unknown source %s", owner)
	}
	return task, backup
}

func TestSharedDestinationDeletesOnlyUnmatchedRepos(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 0), testTarget("second", 0))
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backup.deleted, []string{"stale"}) {
		t.Fatalf("deleted = %v, want only stale", backup.deleted)
	}
	if !slices.Contains(backup.repos, "one") || !slices.Contains(backup.repos, "two") {
		t.Fatalf("source repos missing from destination: %v", backup.repos)
	}
}

func TestSharedDestinationSkipsDeletionOnSourceFailure(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 0), testTarget("second", 0))
	task.loadSource = func(_ context.Context, _, owner string, _ bool) ([]github.Repo, error) {
		if owner == "second" {
			return nil, errors.New("source unavailable")
		}
		return []github.Repo{{Name: "one"}}, nil
	}
	if err := task.Run(context.Background()); err == nil {
		t.Fatal("expected source error")
	}
	if len(backup.deleted) != 0 {
		t.Fatalf("deleted after source failure: %v", backup.deleted)
	}
}

func TestSharedDestinationSkipsDeletionOnMigrationFailure(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 0), testTarget("second", 0))
	backup.migrateErr = map[string]error{"first/one": errors.New("migration failed")}
	if err := task.Run(context.Background()); err == nil {
		t.Fatal("expected migration error")
	}
	if len(backup.deleted) != 0 {
		t.Fatalf("deleted after migration failure: %v", backup.deleted)
	}
}

func TestSharedDestinationHonorsIgnoreTarget(t *testing.T) {
	ignored := testTarget("second", 0)
	ignored.Filter.UnmatchedRepoAction = config.UnmatchedRepoActionIgnore
	ignored.Filter.DenyRule = []string{"second/two/"}
	task, backup := testTask(t, testTarget("first", 0), ignored)
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backup.deleted, []string{"stale"}) {
		t.Fatalf("deleted = %v, want only stale", backup.deleted)
	}
}

func TestInvalidRuleFailsBeforeDeletion(t *testing.T) {
	second := testTarget("second", 0)
	second.Filter.AllowRule = []string{"["}
	task, backup := testTask(t, testTarget("first", 0), second)
	if err := task.Run(context.Background()); err == nil {
		t.Fatal("expected invalid rule error")
	}
	if len(backup.deleted) != 0 {
		t.Fatalf("deleted with invalid rule: %v", backup.deleted)
	}
}

func TestSharedDestinationDeletionCheckCount(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 2), testTarget("second", 2))
	for run := range 2 {
		if err := task.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(backup.deleted) != 0 {
			t.Fatalf("deleted after run %d: %v", run+1, backup.deleted)
		}
	}
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backup.deleted, []string{"stale"}) {
		t.Fatalf("deleted = %v, want stale after three runs", backup.deleted)
	}
}

func TestTokenRulesUseStablePriority(t *testing.T) {
	target := testTarget("first", 0)
	target.Filter.UnmatchedRepoAction = config.UnmatchedRepoActionIgnore
	target.SpecificGithubToken = map[string]string{
		"first/one":   "second",
		"^first/one/": "first",
	}
	task, backup := testTask(t, target)
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backup.migrated["first/one"] != "first" {
		t.Fatalf("selected token = %q", backup.migrated["first/one"])
	}
}

func TestGiteaRepoNamesAreMatchedCaseInsensitively(t *testing.T) {
	target := testTarget("first", 0)
	target.Backup.Type = config.BackupProviderConfigTypeGitea
	task, backup := testTask(t, target)
	backup.repos = []string{"ONE", "stale"}
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backup.deleted, []string{"stale"}) {
		t.Fatalf("deleted = %v, want only stale", backup.deleted)
	}
}

func TestCancelledRunDoesNotDelete(t *testing.T) {
	task, backup := testTask(t, testTarget("first", 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(task.Run(ctx), context.Canceled) {
		t.Fatal("expected context cancellation")
	}
	if len(backup.deleted) != 0 {
		t.Fatalf("deleted after cancellation: %v", backup.deleted)
	}
}

func TestRunPassesCallerContextToSourceAndProvider(t *testing.T) {
	target := testTarget("first", 0)
	target.Filter.UnmatchedRepoAction = config.UnmatchedRepoActionIgnore
	task, backup := testTask(t, target)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sourceCtx context.Context
	task.loadSource = func(got context.Context, _, _ string, _ bool) ([]github.Repo, error) {
		sourceCtx = got
		return []github.Repo{{Name: "one"}}, nil
	}
	if err := task.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if sourceCtx != ctx || backup.migrateCtx != ctx {
		t.Fatal("run context was not passed to source and backup provider")
	}
}
