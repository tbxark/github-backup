package local

import (
	"context"
	"errors"
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
	if err := gitCloneMirror(context.Background(), source, owner, mirror, ""); err != nil {
		t.Fatal(err)
	}
	bare, err := isBareRepository(context.Background(), mirror)
	if err != nil || !bare {
		t.Fatalf("mirror is not bare: %v", err)
	}
	first := git(t, mirror, "rev-parse", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(work, "file"), []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "commit", "-am", "second")
	git(t, work, "push", "origin", "main")
	if err := gitFetchMirror(context.Background(), mirror, ""); err != nil {
		t.Fatal(err)
	}
	second := git(t, mirror, "rev-parse", "refs/heads/main")
	if first == second {
		t.Fatal("mirror did not update")
	}
}

func TestCloneMarkerFailureLeavesNoDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.git")
	owner := filepath.Join(root, "backup")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", source)
	destination := filepath.Join(owner, "repo")
	err := cloneRepository(context.Background(), source, owner, destination, "", true, func(string) error {
		return errors.New("marker failed")
	})
	if err == nil {
		t.Fatal("expected marker failure")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination remains after marker failure: %v", err)
	}
}

func TestLoadReposSkipsDirectoriesInsideParentRepository(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	ownerPath := filepath.Join(root, "owner")
	if err := os.MkdirAll(filepath.Join(ownerPath, "unrelated"), 0700); err != nil {
		t.Fatal(err)
	}
	mirror := filepath.Join(ownerPath, "mirror")
	git(t, root, "init", "--bare", mirror)
	git(t, mirror, "remote", "add", "origin", "https://github.com/source/mirror.git")
	git(t, mirror, "config", "remote.origin.mirror", "true")
	if err := markManagedRepo(context.Background(), mirror, "source", "mirror"); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", filepath.Join(ownerPath, "unmanaged"))
	client := NewLocal(&Config{Root: root})
	repos, err := client.LoadRepos(context.Background(), &provider.Owner{Name: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != "mirror" {
		t.Fatalf("unexpected repositories: %v", repos)
	}
}

func TestDeleteRepoRejectsUnmanagedAndTraversal(t *testing.T) {
	root := t.TempDir()
	ownerPath := filepath.Join(root, "owner")
	if err := os.Mkdir(ownerPath, 0700); err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(ownerPath, "unmanaged")
	git(t, root, "init", "--bare", unmanaged)
	client := NewLocal(&Config{Root: root})
	if _, err := client.DeleteRepo(context.Background(), "owner", "unmanaged"); err == nil {
		t.Fatal("unmanaged repository was deleted")
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatal("unmanaged repository is missing:", err)
	}
	if _, err := client.DeleteRepo(context.Background(), "..", "unmanaged"); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestDeleteRepoRemovesManagedRepository(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner", "repo")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", path)
	git(t, path, "remote", "add", "origin", "https://github.com/source/repo.git")
	git(t, path, "config", "remote.origin.mirror", "true")
	if err := markManagedRepo(context.Background(), path, "source", "repo"); err != nil {
		t.Fatal(err)
	}
	client := NewLocal(&Config{Root: root})
	if _, err := client.DeleteRepo(context.Background(), "owner", "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("managed repository still exists: %v", err)
	}
}

func TestDeleteRepoRejectsSymlinkAndChangedOrigin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner", "repo")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", path)
	git(t, path, "remote", "add", "origin", "https://github.com/source/repo.git")
	git(t, path, "config", "remote.origin.mirror", "true")
	if err := markManagedRepo(context.Background(), path, "source", "repo"); err != nil {
		t.Fatal(err)
	}
	client := NewLocal(&Config{Root: root})
	git(t, path, "remote", "set-url", "origin", "https://github.com/other/repo.git")
	if _, err := client.DeleteRepo(context.Background(), "owner", "repo"); err == nil {
		t.Fatal("repository with changed origin was deleted")
	}
	if err := os.Symlink(path, filepath.Join(root, "owner", "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteRepo(context.Background(), "owner", "link"); err == nil {
		t.Fatal("repository symlink was accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("repository is missing:", err)
	}
}

func TestMigrateRejectsMirrorWithDifferentOrigin(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "dest", "repo")
	if err := os.MkdirAll(filepath.Dir(repoPath), 0700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--bare", repoPath)
	git(t, repoPath, "remote", "add", "origin", "https://github.com/other/repo.git")
	git(t, repoPath, "config", "remote.origin.mirror", "true")
	client := NewLocal(&Config{Root: root})
	_, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"})
	if err == nil || !strings.Contains(err.Error(), "not a mirror of") {
		t.Fatalf("expected origin mismatch, got %v", err)
	}
}

func TestVerifyMirrorOriginAcceptsExpectedMirror(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	git(t, root, "init", "--bare", repoPath)
	git(t, repoPath, "remote", "add", "origin", "https://github.com/source/repo.git")
	git(t, repoPath, "config", "remote.origin.mirror", "true")
	if err := verifyMirrorOrigin(context.Background(), repoPath, "https://github.com/source/repo.git"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkingTreeCloneAndPull(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.git")
	work := filepath.Join(root, "work")
	owner := filepath.Join(root, "backup")
	clone := filepath.Join(owner, "repo")
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
	git(t, source, "symbolic-ref", "HEAD", "refs/heads/main")
	if err := gitCloneWorktree(context.Background(), source, owner, clone, ""); err != nil {
		t.Fatal(err)
	}
	bare, err := isBareRepository(context.Background(), clone)
	if err != nil || bare {
		t.Fatalf("clone should be a working tree: bare=%v err=%v", bare, err)
	}
	if err := os.WriteFile(filepath.Join(work, "file"), []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "commit", "-am", "second")
	git(t, work, "push", "origin", "main")
	if err := gitUpdateWorktree(context.Background(), clone, "", UpdateActionPull); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(clone, "file"))
	if err != nil || string(content) != "two" {
		t.Fatalf("pull content = %q, error = %v", content, err)
	}
}

func TestWorktreeOriginAcceptsHTTPSAndSSH(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	for index, remote := range []string{
		"https://github.com/source/repo.git",
		"git@github.com:source/repo.git",
		"ssh://git@github.com/source/repo.git",
	} {
		if index == 0 {
			git(t, root, "remote", "add", "origin", remote)
		} else {
			git(t, root, "remote", "set-url", "origin", remote)
		}
		if err := verifyWorktreeOrigin(context.Background(), root, "https://github.com/source/repo.git"); err != nil {
			t.Fatalf("remote %q: %v", remote, err)
		}
	}
}

func TestExistingWorktreeRequiresAction(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "dest", "repo")
	if err := os.MkdirAll(repoPath, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repoPath, "init")
	client := NewLocal(&Config{Root: root})
	_, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"})
	if err == nil || !strings.Contains(err.Error(), "set local action") {
		t.Fatalf("expected action guidance, got %v", err)
	}
}

func TestMigrateExistingWorktreePulls(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "upstream.git")
	work := filepath.Join(root, "work")
	destination := filepath.Join(root, "dest", "repo")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
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
	git(t, source, "symbolic-ref", "HEAD", "refs/heads/main")
	git(t, root, "clone", source, destination)
	remote := "https://github.com/source/repo.git"
	git(t, destination, "remote", "set-url", "origin", remote)
	git(t, destination, "config", "url.file://"+source+".insteadOf", remote)
	if err := os.WriteFile(filepath.Join(work, "file"), []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "commit", "-am", "second")
	git(t, work, "push", "origin", "main")
	client := NewLocal(&Config{Root: root, Action: UpdateActionPull})
	if _, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "file"))
	if err != nil || string(content) != "two" {
		t.Fatalf("migrated content = %q, error = %v", content, err)
	}
}

func TestQuestionsDoNotBlockNonInteractiveRun(t *testing.T) {
	client := NewLocal(&Config{Root: t.TempDir(), Questions: true})
	client.SetInteractive(false)
	if _, err := client.MigrateRepo(context.Background(), &provider.Owner{Name: "source"}, &provider.Owner{Name: "dest"}, &provider.Repo{Name: "repo"}); err == nil {
		t.Fatal("migration prompted in non-interactive mode")
	}
	if _, err := client.DeleteRepo(context.Background(), "dest", "repo"); err == nil {
		t.Fatal("deletion prompted in non-interactive mode")
	}
}
