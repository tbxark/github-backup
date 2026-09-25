package local

import (
	"encoding/base64"
	"fmt"
	"log"
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
	conf *Config
}

func NewLocal(conf *Config) *Local {
	return &Local{conf: conf}
}

func (l *Local) LoadRepos(owner *provider.Owner) ([]string, error) {
	ownerPath := filepath.Join(l.conf.Root, owner.Name)
	dirEntries, err := os.ReadDir(ownerPath)
	if err != nil {
		return nil, err
	}
	repos := make([]string, 0)
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			if !isGitRepository(filepath.Join(ownerPath, dirEntry.Name())) {
				log.Printf("skipping non-git dir %s/%s", owner.Name, dirEntry.Name())
				continue
			}
			repos = append(repos, dirEntry.Name())
		}
	}
	return repos, nil
}

func (l *Local) MigrateRepo(from *provider.Owner, to *provider.Owner, repo *provider.Repo) (string, error) {
	if l.conf.Questions && !question(fmt.Sprintf("Are you sure you want to migrate %s/%s to %s/%s? [y/n]: ", from.Name, repo.Name, to.Name, repo.Name)) {
		return "skip", nil
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
			if err := gitCloneMirror(gitURL, ownerPath, repoPath, repo.AuthToken); err != nil {
				return "", err
			}
			return "success", nil
		} else {
			return "", err
		}
	} else {
		bare, err := isBareRepository(repoPath)
		if err != nil {
			return "", err
		}
		if !bare {
			return "", fmt.Errorf("%s is a legacy working-tree clone; move it aside before creating a mirror backup", repoPath)
		}
	}
	if err := gitFetchMirror(repoPath, repo.AuthToken); err != nil {
		return "", err
	}
	return "success", nil
}

func (l *Local) DeleteRepo(owner, repo string) (string, error) {
	if l.conf.Questions && !question(fmt.Sprintf("Are you sure you want to delete %s/%s? [y/n]: ", owner, repo)) {
		return "skip", nil
	}
	repoPath := filepath.Join(l.conf.Root, owner, repo)
	err := os.RemoveAll(repoPath)
	if err != nil {
		return "fail", err
	}
	return "success", nil
}

func isGitRepository(path string) bool {
	bare, err := isBareRepository(path)
	if err != nil {
		return false
	}
	arg := "--show-toplevel"
	if bare {
		arg = "--absolute-git-dir"
	}
	cmd := exec.Command("git", "rev-parse", arg)
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

func isBareRepository(path string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--is-bare-repository")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("inspect Git repository %s: %w", path, err)
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

func gitCloneMirror(url, ownerPath, path, token string) error {
	log.Printf("cloning mirror %s", url)
	tmp, err := os.MkdirTemp(ownerPath, ".github-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	mirrorPath := filepath.Join(tmp, "mirror")
	cmd := exec.Command("git", "clone", "--mirror", url, mirrorPath)
	cmd.Env = gitEnvironment(token)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone mirror: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return os.Rename(mirrorPath, path)
}

func gitFetchMirror(path, token string) error {
	log.Printf("fetching mirror %s", path)
	cmd := exec.Command("git", "remote", "update", "--prune", "origin")
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

func question(message string) bool {
	var response string
	fmt.Print(message)
	_, err := fmt.Scanln(&response)
	if err != nil {
		return false
	}
	return response == "y"
}
