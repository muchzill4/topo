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

	validator, err := NewCatalogSchema()
	if err != nil {
		log.Fatalf("failed to create schema validator: %v\n", err)
	}

	sources, err := ListGitHubSources()
	if err != nil {
		log.Fatalf("failed to list sources: %v\n", err)
	}

	var templates []Template
	for _, source := range sources {
		template, err := FetchTemplate(githubClient, source)
		if err != nil {
			log.Printf("failed to fetch %s (%v)\n", source, err)
			continue
		}
		if err := validator.Validate(template); err != nil {
			log.Printf("invalid template %s (%v)\n", source, err)
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
