package stats

import "testing"

func TestAggregateExcludesExternalByDefault(t *testing.T) {
	collected := &CollectedStats{
		Name:               "Test User",
		TotalContributions: 42,
		Repositories: []Repository{
			{
				Name:         "me/owned",
				Stars:        5,
				Forks:        2,
				Views:        10,
				LinesChanged: 100,
				Source:       RepositoryOwned,
				Languages:    []Language{{Name: "Go", Size: 300, Color: "#00ADD8"}},
			},
			{
				Name:         "org/collab",
				Stars:        7,
				Forks:        3,
				Views:        20,
				LinesChanged: 200,
				Source:       RepositoryCollaborator,
				Languages:    []Language{{Name: "Go", Size: 100, Color: "#00ADD8"}},
			},
			{
				Name:         "other/external",
				Stars:        100,
				Forks:        50,
				Views:        30,
				LinesChanged: 300,
				Source:       RepositoryExternalContributor,
				Languages:    []Language{{Name: "Rust", Size: 500, Color: "#dea584"}},
			},
		},
	}

	summary := Aggregate(collected, AggregateOptions{})
	if summary.Stars != 12 || summary.Forks != 5 {
		t.Fatalf("unexpected engagement totals: %+v", summary)
	}
	if summary.RepositoryCount != 2 {
		t.Fatalf("expected 2 included repositories, got %d", summary.RepositoryCount)
	}
	if len(summary.Languages) != 1 || summary.Languages[0].Name != "Go" {
		t.Fatalf("expected only Go to remain, got %+v", summary.Languages)
	}
}

func TestAggregateCanIncludeExternalRepositories(t *testing.T) {
	collected := &CollectedStats{
		Name: "Test User",
		Repositories: []Repository{
			{Name: "me/owned", Stars: 1, Forks: 1, Source: RepositoryOwned},
			{Name: "other/external", Stars: 10, Forks: 5, Source: RepositoryExternalContributor},
		},
	}

	summary := Aggregate(collected, AggregateOptions{IncludeExternalRepositories: true})
	if summary.Stars != 11 || summary.Forks != 6 || summary.RepositoryCount != 2 {
		t.Fatalf("unexpected totals when including external repositories: %+v", summary)
	}
}

func TestAggregateSupportsRepoAndPrivateExclusions(t *testing.T) {
	collected := &CollectedStats{
		Name: "Test User",
		Repositories: []Repository{
			{Name: "me/owned", Stars: 1, Source: RepositoryOwned},
			{Name: "me/private", Stars: 10, Private: true, Source: RepositoryOwned},
			{Name: "org/ignore-me", Stars: 20, Source: RepositoryCollaborator},
		},
	}

	summary := Aggregate(collected, AggregateOptions{
		ExcludePrivate:      true,
		ExcludeRepoPatterns: []string{"org/*"},
	})

	if summary.Stars != 1 || summary.RepositoryCount != 1 {
		t.Fatalf("unexpected totals after exclusions: %+v", summary)
	}
}

func TestSanitizeRepositoryNameRedactsPrivateRepositories(t *testing.T) {
	if got := sanitizeRepositoryName("org/secret", true); got != "<private>" {
		t.Fatalf("expected private repository name to be redacted, got %q", got)
	}
}

func TestSanitizeGitCommandOutputRedactsPrivateRepositoryAndToken(t *testing.T) {
	repository := Repository{
		Name:          "org/secret",
		SanitizedName: sanitizeRepositoryName("org/secret", true),
		Private:       true,
	}

	got := sanitizeGitCommandOutput(
		"fatal: could not read from https://x-access-token:top-secret@github.com/org/secret.git",
		repository,
		"top-secret",
	)
	if got != "fatal: could not read from https://x-access-token:<redacted-token>@github.com/<private>.git" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}
