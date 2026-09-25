package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbxark/github-backup/provider/provider"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestMirrorCloneAndUpdate(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.git")
	work := filepath.Join(root, "work")
	owner := filepath.Join(root, "backup")
	mirror := filepath.Join(owner, "repo")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", source)
	git(t, root, "clone", source, work)
	git(t, work, "checkout", "-b", "main")
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "file"), []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "file")
	git(t, work, "commit", "-m", "first")
	git(t, work, "push", "origin", "main")
	if err := gitCloneMirror(source, owner, mirror, ""); err != nil {
		t.Fatal(err)
	}
	bare, err := isBareRepository(mirror)
	if err != nil || !bare {
		t.Fatalf("mirror is not bare: %v", err)
	}
	first := git(t, mirror, "rev-parse", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(work, "file"), []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "commit", "-am", "second")
	git(t, work, "push", "origin", "main")
	if err := gitFetchMirror(mirror, ""); err != nil {
		t.Fatal(err)
	}
	second := git(t, mirror, "rev-parse", "refs/heads/main")
	if first == second {
		t.Fatal("mirror did not update")
	}
}

func TestLoadReposSkipsDirectoriesInsideParentRepository(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	ownerPath := filepath.Join(root, "owner")
	if err := os.MkdirAll(filepath.Join(ownerPath, "unrelated"), 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", filepath.Join(ownerPath, "mirror"))
	client := NewLocal(&Config{Root: root})
	repos, err := client.LoadRepos(&provider.Owner{Name: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != "mirror" {
		t.Fatalf("unexpected repositories: %v", repos)
	}
}
