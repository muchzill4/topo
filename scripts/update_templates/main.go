package main

import (
	"log"
	"os"
)

func main() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Println("⚠️ GITHUB_TOKEN is not set: you might get rate limited")
	}

	githubClient := NewGitHubClient(githubToken)

	sourcesFile, err := openGitHubSources()
	if err != nil {
		log.Fatalf("failed to open GitHub sources: %v\n", err)
	}
	defer sourcesFile.Close() //nolint:errcheck // Closing a read-only file cannot affect catalog generation.

	var templates []Template
	for _, source := range ListGitHubSources(sourcesFile) {
		template, err := FetchTemplate(githubClient, source)
		if err != nil {
			log.Printf("failed to fetch %s (%v)\n", source, err)
			continue
		}
		log.Printf("fetched %s\n", source)
		templates = append(templates, template)
	}

	filePath, err := WriteCatalogFile(templates)
	if err != nil {
		log.Fatalf("failed to write catalog file: %v\n", err)
	}
	log.Printf("written catalog to %s\n", filePath)
}
