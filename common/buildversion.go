package common

import (
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
)

func GetCommitHash() string {
	if cwd, err := os.Getwd(); err == nil {
		if hash := computeHashFromPath(cwd); hash != "" {
			if len(hash) >= 8 {
				return hash[:8]
			}
			return hash
		}
	}

	if exePath, err := os.Executable(); err == nil {
		repoPath := filepath.Dir(exePath)
		if hash := computeHashFromPath(repoPath); hash != "" {
			if len(hash) >= 8 {
				return hash[:8]
			}
			return hash
		}
	}

	return "unknown"
}

func computeHashFromPath(path string) string {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head.Hash().String()
}

func GetJAMNetwork() string {
	if val := os.Getenv("JAM_NETWORK"); val != "" {
		return val
	}
	return "jam"
}

func GetJAMNetworkPort() int {
	if val := os.Getenv("JAM_NETWORK"); val != "" {
		return 9800
	}
	return 9800
}

func GetAddresses(local bool, port int) []string {
	addresses := make([]string, 6)
	if local {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("localhost:%d", port+i)
		}
	} else {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("%s-%d.jamduna.org:%d", GetJAMNetwork(), i, port)
		}
	}
	return addresses
}
