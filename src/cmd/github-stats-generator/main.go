package main

import (
	"context"
	"log"
	"os"

	"github.com/tuunit/github-stats-generator/internal/config"
	"github.com/tuunit/github-stats-generator/internal/githubapi"
	"github.com/tuunit/github-stats-generator/internal/render"
	"github.com/tuunit/github-stats-generator/internal/stats"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("github-stats-generator: ")

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Waking up the contribution vanity engine. Output goes to %q.", cfg.OutputDir)

	client := githubapi.NewClient(cfg.AccessToken)
	service := stats.NewService(client, cfg.MaxStatsRetries)

	collected, err := service.Collect(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	summary := stats.Aggregate(collected, stats.AggregateOptions{
		ExcludeRepoPatterns:         cfg.ExcludeRepoPatterns,
		ExcludeLanguagePatterns:     cfg.ExcludeLanguagePatterns,
		ExcludePrivate:              cfg.ExcludePrivate,
		IncludeExternalRepositories: cfg.IncludeContributedRepositories,
	})
	log.Printf(
		"Scoreboard ready: %d repo(s), %d stars, %d forks, %d views, %d lines changed.",
		summary.RepositoryCount,
		summary.Stars,
		summary.Forks,
		summary.Views,
		summary.LinesChanged,
	)

	if err := render.WriteOutput(cfg.OutputDir, summary); err != nil {
		log.Fatal(err)
	}
	log.Printf("Artifacts written. Your contribution marketing department can clock out.")
}
