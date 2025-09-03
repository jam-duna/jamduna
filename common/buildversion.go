package common

import (
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
)

func GetJAMTestVectorPath(subDir ...string) string {
	p := os.Getenv("JAM_TESTVECTORS_PATH")
	if p == "" {
		p = "/Users/michael/Desktop/jam-test-vectors"
	}
	absPath, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}
	targetDir := "stf" // Default sub dir
	if len(subDir) > 0 {
		targetDir = subDir[0]
	}
	return filepath.Join(absPath, targetDir)
}

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
