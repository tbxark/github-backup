package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const managedSourceKey = "github-backup.source"

func validPathSegment(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!filepath.IsAbs(name) && !strings.ContainsAny(name, "/\\")
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && filepath.IsLocal(relative)
}

func (l *Local) ownerPath(owner string) (string, error) {
	if !validPathSegment(owner) {
		return "", fmt.Errorf("invalid backup owner %q", owner)
	}
	root, err := filepath.Abs(l.conf.Root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	} else if !os.IsNotExist(err) {
		return "", err
	}
	path := filepath.Join(l.conf.Root, owner)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("backup owner path %s is a symlink", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if !within(root, resolved) {
			return "", fmt.Errorf("backup owner %s escapes root %s", owner, l.conf.Root)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (l *Local) repoPath(owner, repo string) (string, error) {
	if !validPathSegment(repo) {
		return "", fmt.Errorf("invalid backup repo %q", repo)
	}
	ownerPath, err := l.ownerPath(owner)
	if err != nil {
		return "", err
	}
	path := filepath.Join(ownerPath, repo)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("backup repo path %s is a symlink", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		parent, err := filepath.EvalSymlinks(ownerPath)
		if err != nil {
			return "", err
		}
		if !within(parent, resolved) {
			return "", fmt.Errorf("backup repo %s escapes owner %s", repo, owner)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func markManagedRepo(ctx context.Context, path, sourceOwner, repo string) error {
	if !validPathSegment(sourceOwner) || !validPathSegment(repo) {
		return fmt.Errorf("invalid source repository %q/%q", sourceOwner, repo)
	}
	source := strings.ToLower(sourceOwner + "/" + repo)
	cmd := exec.CommandContext(ctx, "git", "config", "--local", managedSourceKey, source)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mark managed repository %s: %w: %s", path, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func isManagedRepo(ctx context.Context, path, repo string) bool {
	cmd := exec.CommandContext(ctx, "git", "config", "--local", "--get", managedSourceKey)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	source := strings.TrimSpace(string(output))
	parts := strings.Split(source, "/")
	if len(parts) != 2 || !validPathSegment(parts[0]) || !strings.EqualFold(parts[1], repo) {
		return false
	}
	expected := "https://github.com/" + source + ".git"
	bare, err := isBareRepository(ctx, path)
	if err != nil {
		return false
	}
	if bare {
		return verifyMirrorOrigin(ctx, path, expected) == nil
	}
	return verifyWorktreeOrigin(ctx, path, expected) == nil
}
