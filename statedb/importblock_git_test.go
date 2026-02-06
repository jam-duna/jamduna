package statedb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ConformanceSource struct {
	Path   string
	Branch string // empty means use current branch (not a git repo or no branch switching)
	Name   string // display name for test output
}

func FetchRemoteBranches(repoPath string) {
	fmt.Printf("Fetching remote branches from %s...\n", repoPath)
	cmd := exec.Command("git", "-C", repoPath, "fetch", "--all", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Warning: failed to fetch remote branches: %v\n%s\n", err, string(output))
		return
	}
	fmt.Printf("Fetch complete.\n\n")
}

func GetW3FBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "branch", "-r")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remote branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "->") {
			continue // Skip empty lines and HEAD pointer (origin/HEAD -> origin/main)
		}
		// Remote branch format: "origin/branch_name" -> extract just branch_name
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			continue
		}
		branch := parts[1]
		branch = strings.TrimSpace(branch)
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// GetGitInfo returns the current branch, short commit hash, and commit date for a repo
func GetGitInfo(repoPath string) (branch, commitHash, commitDate string) {
	// Get current branch
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if output, err := cmd.Output(); err == nil {
		branch = strings.TrimSpace(string(output))
	} else {
		branch = "unknown"
	}

	// Get short commit hash
	cmd = exec.Command("git", "-C", repoPath, "rev-parse", "--short", "HEAD")
	if output, err := cmd.Output(); err == nil {
		commitHash = strings.TrimSpace(string(output))
	} else {
		commitHash = "unknown"
	}

	// Get commit date and time
	cmd = exec.Command("git", "-C", repoPath, "log", "-1", "--format=%ci")
	if output, err := cmd.Output(); err == nil {
		// Format: "2025-01-27 10:30:00 +0000" -> take date and time "2025-01-27 10:30:00"
		dateStr := strings.TrimSpace(string(output))
		if len(dateStr) >= 19 {
			commitDate = dateStr[:19] // YYYY-MM-DD HH:MM:SS
		} else {
			commitDate = dateStr
		}
	} else {
		commitDate = "unknown"
	}

	return branch, commitHash, commitDate
}

// PrintGitStatus prints a formatted banner showing the current git status of a repo
func PrintGitStatus(repoPath, label string) {
	branch, commitHash, commitDate := GetGitInfo(repoPath)
	fmt.Printf("\n")
	fmt.Printf("========================================\n")
	fmt.Printf("  %s\n", label)
	fmt.Printf("  Branch: %s (commit %s - %s)\n", branch, commitHash, commitDate)
	fmt.Printf("  Repo: %s\n", repoPath)
	fmt.Printf("========================================\n")
	fmt.Printf("\n")
}

// CheckoutBranch switches to the specified branch in the given repo
// It checks out the remote branch (origin/branch) to ensure we have the latest
func CheckoutBranch(repoPath, branch string) error {
	// Checkout the remote tracking branch directly to get latest
	remoteRef := fmt.Sprintf("origin/%s", branch)
	cmd := exec.Command("git", "-C", repoPath, "checkout", remoteRef)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: try checking out local branch
		cmd = exec.Command("git", "-C", repoPath, "checkout", branch)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to checkout branch %s: %w\n%s", branch, err, string(output))
		}
	}
	// Get commit info after checkout
	_, commitHash, commitDate := GetGitInfo(repoPath)
	fmt.Printf("\n")
	fmt.Printf("========================================\n")
	fmt.Printf("  SWITCHED TO BRANCH: %s (commit %s - %s)\n", branch, commitHash, commitDate)
	fmt.Printf("  Repo: %s\n", repoPath)
	fmt.Printf("========================================\n")
	fmt.Printf("\n")
	return nil
}

// GetFuzzReportsPaths returns paths for all configured conformance sources
func GetFuzzReportsPaths(subDir ...string) ([]string, error) {
	sources, err := GetConformanceSources(false, subDir...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, src := range sources {
		paths = append(paths, src.Path)
	}
	return paths, nil
}

// GetConformanceSources returns conformance sources.
// If loopAllBranches is true, it will return all branches for W3F repo.
// If loopAllBranches is false, it uses the current branch without switching.
func GetConformanceSources(loopAllBranches bool, subDir ...string) ([]ConformanceSource, error) {
	var sources []ConformanceSource
	targetDir := ""
	if len(subDir) > 0 {
		targetDir = subDir[0]
	}

	// Check W3F path (primary)
	if w3fPath := os.Getenv("JAM_CONFORMANCE_W3F_PATH"); w3fPath != "" {
		absPath, err := filepath.Abs(filepath.Join(w3fPath, targetDir))
		if err != nil {
			fmt.Printf("Warning: failed to resolve JAM_CONFORMANCE_W3F_PATH: %v\n", err)
		} else if _, err := os.Stat(absPath); os.IsNotExist(err) {
			fmt.Printf("Warning: JAM_CONFORMANCE_W3F_PATH does not exist: %s\n", absPath)
		} else {
			if loopAllBranches {
				FetchRemoteBranches(w3fPath)

				branches, err := GetW3FBranches(w3fPath)
				if err != nil {
					fmt.Printf("Warning: failed to get W3F branches: %v\n", err)
					PrintGitStatus(w3fPath, "JAM_CONFORMANCE_W3F_PATH (fallback to current)")
					sources = append(sources, ConformanceSource{
						Path: absPath,
						Name: "w3f-current",
					})
				} else {
					fmt.Printf("JAM_CONFORMANCE_W3F_PATH: %s\n", absPath)
					fmt.Printf("Will test branches: %v\n", branches)
					for _, branch := range branches {
						sources = append(sources, ConformanceSource{
							Path:   absPath,
							Branch: branch,
							Name:   fmt.Sprintf("w3f-%s", branch),
						})
					}
				}
			} else {
				PrintGitStatus(w3fPath, "JAM_CONFORMANCE_W3F_PATH (using current branch)")
				sources = append(sources, ConformanceSource{
					Path: absPath,
					Name: "w3f",
				})
			}
		}
	}

	requireSecondary := false
	if len(sources) == 0 {
		requireSecondary = true
	}
	if requireSecondary {
		if fuzzPath := os.Getenv("JAM_CONFORMANCE_PATH"); fuzzPath != "" {
			absPath, err := filepath.Abs(filepath.Join(fuzzPath, targetDir))
			if err != nil {
				fmt.Printf("Warning: failed to resolve JAM_CONFORMANCE_PATH: %v\n", err)
			} else if _, err := os.Stat(absPath); os.IsNotExist(err) {
				fmt.Printf("Warning: JAM_CONFORMANCE_PATH does not exist: %s\n", absPath)
			} else {
				PrintGitStatus(fuzzPath, "JAM_CONFORMANCE_PATH")
				sources = append(sources, ConformanceSource{
					Path: absPath,
					Name: "jam-conformance",
				})
			}
		}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no valid conformance paths found (check JAM_CONFORMANCE_W3F_PATH and/or JAM_CONFORMANCE_PATH)")
	}

	return sources, nil
}

// GetFuzzReportsPath returns the first configured conformance path (for backwards compatibility).
// Deprecated: Use GetFuzzReportsPaths for multi-source testing.
func GetFuzzReportsPath(subDir ...string) (string, error) {
	paths, err := GetFuzzReportsPaths(subDir...)
	if err != nil {
		return "", err
	}
	return paths[0], nil
}
