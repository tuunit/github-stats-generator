package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AccessToken                    string
	OutputDir                      string
	ExcludeRepoPatterns            []string
	ExcludeLanguagePatterns        []string
	ExcludePrivate                 bool
	IncludeContributedRepositories bool
	MaxStatsRetries                int
}

func Load(args []string) (Config, error) {
	cfg := Config{}
	fs := flag.NewFlagSet("github-stats-generator", flag.ContinueOnError)

	defaultAccessToken := envFirst("ACCESS_TOKEN")
	defaultOutputDir := envFirst("OUTPUT_DIR")
	if defaultOutputDir == "" {
		defaultOutputDir = "."
	}
	defaultExcludeRepos := envFirst("EXCLUDE_REPOS", "EXCLUDED")
	defaultExcludeLangs := envFirst("EXCLUDE_LANGS", "EXCLUDED_LANGS")
	defaultExcludePrivate := envBool("EXCLUDE_PRIVATE", false)
	defaultIncludeContributed := includeContributedDefault()
	defaultMaxRetries := envInt("MAX_RETRIES", 5)

	fs.StringVar(&cfg.AccessToken, "access-token", defaultAccessToken, "GitHub personal access token")
	fs.StringVar(&cfg.OutputDir, "output-dir", defaultOutputDir, "Directory to write generated output files into")
	excludeRepos := fs.String("exclude-repos", defaultExcludeRepos, "Comma-separated repository patterns to exclude")
	excludeLangs := fs.String("exclude-langs", defaultExcludeLangs, "Comma-separated language patterns to exclude")
	fs.BoolVar(&cfg.ExcludePrivate, "exclude-private", defaultExcludePrivate, "Exclude private repositories from aggregate output")
	fs.BoolVar(&cfg.IncludeContributedRepositories, "include-contributed-repositories", defaultIncludeContributed, "Include externally contributed repositories in aggregate output")
	fs.IntVar(&cfg.MaxStatsRetries, "max-retries", defaultMaxRetries, "Number of retries before falling back to git log for contributor stats")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.AccessToken == "" {
		return Config{}, fmt.Errorf("an access token is required; set ACCESS_TOKEN or pass --access-token")
	}

	cfg.ExcludeRepoPatterns = splitList(*excludeRepos)
	cfg.ExcludeLanguagePatterns = splitList(*excludeLangs)
	return cfg, nil
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func includeContributedDefault() bool {
	if value, ok := os.LookupEnv("INCLUDE_CONTRIBUTED_REPOSITORIES"); ok && strings.TrimSpace(value) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}

	if value, ok := os.LookupEnv("EXCLUDE_FORKED_REPOS"); ok && strings.TrimSpace(value) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return !parsed
		}
	}

	return false
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	splitter := func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '\t', '|':
			return true
		default:
			return false
		}
	}

	parts := strings.FieldsFunc(value, splitter)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(part, " \"'")
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
