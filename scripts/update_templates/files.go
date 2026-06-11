package main

import (
	"os"
	"path/filepath"
)

const relativeSourcesPath = "scripts/update_templates/sources.json"

func openGitHubSources() (*os.File, error) {
	repoRoot, err := findModuleRoot()
	if err != nil {
		return nil, err
	}

	return os.Open(filepath.Join(repoRoot, filepath.FromSlash(relativeSourcesPath)))
}
