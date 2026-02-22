package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ConflictError is returned when a merge has conflicts.
type ConflictError struct {
	Files []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("merge conflict in %d files: %s", len(e.Files), strings.Join(e.Files, ", "))
}

// RepoRoot returns the git repository root for the given directory.
func RepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel in %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the current branch name for the given directory.
func CurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD in %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// WorktreeAdd creates a new worktree with a new branch.
func WorktreeAdd(repoRoot, worktreeDir, branch string) error {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", branch, worktreeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add -b %s %s: %s: %w", branch, worktreeDir, string(out), err)
	}
	return nil
}

// WorktreeRemove removes a worktree directory.
func WorktreeRemove(repoRoot, worktreeDir string) error {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "remove", "--force", worktreeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove %s: %s: %w", worktreeDir, string(out), err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func DeleteBranch(repoRoot, branch string) error {
	cmd := exec.Command("git", "-C", repoRoot, "branch", "-D", branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %s: %w", branch, string(out), err)
	}
	return nil
}

// MergeNoFF attempts a no-fast-forward merge. Returns the merge commit SHA on success,
// or a *ConflictError if there are conflicts.
func MergeNoFF(dir, branch, baseBranch, message string) (string, error) {
	// Ensure we're on the base branch
	checkout := exec.Command("git", "-C", dir, "checkout", baseBranch)
	if out, err := checkout.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git checkout %s: %s: %w", baseBranch, string(out), err)
	}

	// Attempt merge
	mergeCmd := exec.Command("git", "-C", dir, "merge", "--no-ff", branch, "-m", message)
	out, err := mergeCmd.CombinedOutput()
	if err != nil {
		// Check for conflicts
		outStr := string(out)
		if strings.Contains(outStr, "CONFLICT") || strings.Contains(outStr, "Automatic merge failed") {
			files := conflictFiles(dir)
			return "", &ConflictError{Files: files}
		}
		return "", fmt.Errorf("git merge --no-ff %s: %s: %w", branch, outStr, err)
	}

	// Get the merge commit SHA
	sha, err := revParse(dir, "HEAD")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// MergeSquash performs a squash merge: stages all changes from branch but does not
// create a merge commit. The caller gets a single flat commit on the base branch.
func MergeSquash(dir, branch, baseBranch, message string) (string, error) {
	// Ensure we're on the base branch
	checkout := exec.Command("git", "-C", dir, "checkout", baseBranch)
	if out, err := checkout.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git checkout %s: %s: %w", baseBranch, string(out), err)
	}

	// Squash merge — stages changes but doesn't commit
	squashCmd := exec.Command("git", "-C", dir, "merge", "--squash", branch)
	out, err := squashCmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		if strings.Contains(outStr, "CONFLICT") || strings.Contains(outStr, "Automatic merge failed") {
			files := conflictFiles(dir)
			return "", &ConflictError{Files: files}
		}
		return "", fmt.Errorf("git merge --squash %s: %s: %w", branch, outStr, err)
	}

	// Commit the squashed changes
	commitCmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	sha, err := revParse(dir, "HEAD")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// ListUnmergedBranches returns local branches not yet merged into the given base branch.
func ListUnmergedBranches(dir, baseBranch string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "branch", "--no-merged", baseBranch, "--format", "%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch --no-merged %s: %w", baseBranch, err)
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// ResetHard resets the working tree and index to HEAD.
func ResetHard(dir string) error {
	cmd := exec.Command("git", "-C", dir, "reset", "--hard", "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard HEAD: %s: %w", string(out), err)
	}
	return nil
}

// AbortMerge aborts an in-progress merge.
func AbortMerge(dir string) error {
	cmd := exec.Command("git", "-C", dir, "merge", "--abort")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge --abort: %s: %w", string(out), err)
	}
	return nil
}

// conflictFiles returns the list of files with merge conflicts.
func conflictFiles(dir string) []string {
	cmd := exec.Command("git", "-C", dir, "diff", "--name-only", "--diff-filter=U")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// revParse runs git rev-parse on a ref.
func revParse(dir, ref string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}
