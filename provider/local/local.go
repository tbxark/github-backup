package local

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tbxark/github-backup/provider/provider"
)

type UpdateAction string

const (
	UpdateActionPull  = "pull"
	UpdateActionFetch = "fetch"
)

type Config struct {
	Root      string       `json:"root"`
	Questions bool         `json:"questions"`
	Action    UpdateAction `json:"action"`
}

var _ provider.Provider = &Local{}

type Local struct {
	conf        *Config
	interactive bool
}

func NewLocal(conf *Config) *Local {
	return &Local{conf: conf, interactive: true}
}

func (l *Local) SetInteractive(interactive bool) { l.interactive = interactive }

func (l *Local) RequiresConfirmation() bool { return l.conf.Questions }

func (l *Local) DestinationID() string {
	root, err := filepath.Abs(l.conf.Root)
	if err != nil {
		root = filepath.Clean(l.conf.Root)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return "local:" + root
}

func (l *Local) LoadRepos(ctx context.Context, owner *provider.Owner) ([]string, error) {
	ownerPath := filepath.Join(l.conf.Root, owner.Name)
	dirEntries, err := os.ReadDir(ownerPath)
	if err != nil {
		return nil, err
	}
	repos := make([]string, 0)
	for _, dirEntry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if dirEntry.IsDir() {
			if !isGitRepository(ctx, filepath.Join(ownerPath, dirEntry.Name())) {
				log.Printf("skipping non-git dir %s/%s", owner.Name, dirEntry.Name())
				continue
			}
			repos = append(repos, dirEntry.Name())
		}
	}
	return repos, nil
}

func (l *Local) MigrateRepo(ctx context.Context, from *provider.Owner, to *provider.Owner, repo *provider.Repo) (string, error) {
	if l.conf.Questions {
		if !l.interactive {
			return "", fmt.Errorf("confirmation required to migrate %s/%s in non-interactive mode", from.Name, repo.Name)
		}
		if !question(ctx, fmt.Sprintf("Are you sure you want to migrate %s/%s to %s/%s? [y/n]: ", from.Name, repo.Name, to.Name, repo.Name)) {
			return "skip", nil
		}
	}
	if l.conf.Action != "" && l.conf.Action != UpdateActionPull && l.conf.Action != UpdateActionFetch {
		return "", fmt.Errorf("unsupported action: %s", l.conf.Action)
	}
	ownerPath := filepath.Join(l.conf.Root, to.Name)
	_, err := os.Stat(ownerPath)
	if err != nil {
		if os.IsNotExist(err) {
			if e := os.MkdirAll(ownerPath, 0700); e != nil {
				return "", e
			}
		} else {
			return "", err
		}
	}
	repoPath := filepath.Join(ownerPath, repo.Name)
	_, err = os.Stat(repoPath)
	gitURL := fmt.Sprintf("https://github.com/%s/%s.git", from.Name, repo.Name)
	if err != nil {
		if os.IsNotExist(err) {
			clone := gitCloneMirror
			if l.conf.Action != "" {
				clone = gitCloneWorktree
			}
			if err := clone(ctx, gitURL, ownerPath, repoPath, repo.AuthToken); err != nil {
				return "", err
			}
			return "success", nil
		} else {
			return "", err
		}
	} else {
		bare, err := isBareRepository(ctx, repoPath)
		if err != nil {
			return "", err
		}
		if !bare {
			if l.conf.Action == "" {
				return "", fmt.Errorf("%s is a working-tree clone; set local action to pull or fetch to update it", repoPath)
			}
			if !isGitRepository(ctx, repoPath) {
				return "", fmt.Errorf("%s is not a Git repository root", repoPath)
			}
			if err := verifyWorktreeOrigin(ctx, repoPath, gitURL); err != nil {
				return "", err
			}
			if err := gitUpdateWorktree(ctx, repoPath, repo.AuthToken, l.conf.Action); err != nil {
				return "", err
			}
			return "success", nil
		}
		if err := verifyMirrorOrigin(ctx, repoPath, gitURL); err != nil {
			return "", err
		}
	}
	if err := gitFetchMirror(ctx, repoPath, repo.AuthToken); err != nil {
		return "", err
	}
	return "success", nil
}

func (l *Local) DeleteRepo(ctx context.Context, owner, repo string) (string, error) {
	if l.conf.Questions {
		if !l.interactive {
			return "", fmt.Errorf("confirmation required to delete %s/%s in non-interactive mode", owner, repo)
		}
		if !question(ctx, fmt.Sprintf("Are you sure you want to delete %s/%s? [y/n]: ", owner, repo)) {
			return "skip", nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	repoPath := filepath.Join(l.conf.Root, owner, repo)
	err := os.RemoveAll(repoPath)
	if err != nil {
		return "fail", err
	}
	return "success", nil
}

func isGitRepository(ctx context.Context, path string) bool {
	bare, err := isBareRepository(ctx, path)
	if err != nil {
		return false
	}
	arg := "--show-toplevel"
	if bare {
		arg = "--absolute-git-dir"
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", arg)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	return err == nil && filepath.Clean(strings.TrimSpace(string(output))) == resolvedPath
}

func isBareRepository(ctx context.Context, path string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-bare-repository")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("inspect Git repository %s: %w", path, err)
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

func verifyMirrorOrigin(ctx context.Context, path, expected string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
	cmd.Dir = path
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !strings.EqualFold(strings.TrimSpace(string(output)), expected) {
		return fmt.Errorf("%s is not a mirror of %s", path, expected)
	}
	cmd = exec.CommandContext(ctx, "git", "config", "--bool", "--get", "remote.origin.mirror")
	cmd.Dir = path
	output, err = cmd.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		return fmt.Errorf("%s is not configured as a Git mirror", path)
	}
	return nil
}

func verifyWorktreeOrigin(ctx context.Context, path, expected string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
	cmd.Dir = path
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	actualRepo, actualOK := githubRepoID(strings.TrimSpace(string(output)))
	expectedRepo, expectedOK := githubRepoID(expected)
	if err != nil || !actualOK || !expectedOK || actualRepo != expectedRepo {
		return fmt.Errorf("%s is not a clone of %s", path, expected)
	}
	return nil
}

func githubRepoID(remote string) (string, bool) {
	var repoPath string
	if path, ok := strings.CutPrefix(remote, "git@github.com:"); ok {
		repoPath = path
	} else {
		parsed, err := url.Parse(remote)
		if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
			return "", false
		}
		if parsed.Scheme != "https" && parsed.Scheme != "ssh" {
			return "", false
		}
		repoPath = strings.TrimPrefix(parsed.Path, "/")
	}
	repoPath = strings.TrimSuffix(repoPath, ".git")
	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return strings.ToLower(repoPath), true
}

func gitCloneWorktree(ctx context.Context, url, ownerPath, path, token string) error {
	log.Printf("cloning working tree %s", url)
	tmp, err := os.MkdirTemp(ownerPath, ".github-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	clonePath := filepath.Join(tmp, "clone")
	cmd := exec.CommandContext(ctx, "git", "clone", url, clonePath)
	cmd.Env = gitEnvironment(token)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone working tree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return os.Rename(clonePath, path)
}

func gitUpdateWorktree(ctx context.Context, path, token string, action UpdateAction) error {
	log.Printf("%s working tree %s", action, path)
	cmd := exec.CommandContext(ctx, "git", string(action), "--all")
	cmd.Dir = path
	cmd.Env = gitEnvironment(token)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s working tree: %w: %s", action, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gitCloneMirror(ctx context.Context, url, ownerPath, path, token string) error {
	log.Printf("cloning mirror %s", url)
	tmp, err := os.MkdirTemp(ownerPath, ".github-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	mirrorPath := filepath.Join(tmp, "mirror")
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", url, mirrorPath)
	cmd.Env = gitEnvironment(token)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone mirror: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return os.Rename(mirrorPath, path)
}

func gitFetchMirror(ctx context.Context, path, token string) error {
	log.Printf("fetching mirror %s", path)
	cmd := exec.CommandContext(ctx, "git", "remote", "update", "--prune", "origin")
	cmd.Dir = path
	cmd.Env = gitEnvironment(token)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("update mirror: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gitEnvironment(token string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if token != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+credentials,
		)
	}
	return env
}

func question(ctx context.Context, message string) bool {
	fmt.Print(message)
	answer := make(chan bool, 1)
	go func() {
		var response string
		_, err := fmt.Scanln(&response)
		answer <- err == nil && response == "y"
	}()
	select {
	case result := <-answer:
		return result
	case <-ctx.Done():
		return false
	}
}
