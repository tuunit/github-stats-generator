package render

import (
	"strings"
	"testing"

	"github.com/tuunit/github-stats-generator/internal/stats"
)

func TestRenderOverviewEscapesNameAndFormatsCounts(t *testing.T) {
	summary := stats.Summary{
		Name:            "A&B",
		Stars:           1234,
		Forks:           56,
		Contributions:   78,
		LinesChanged:    91011,
		Views:           12,
		RepositoryCount: 3,
	}

	output := renderOverview("{{ name }} {{ stars }} {{ lines_changed }}", summary)
	if !strings.Contains(output, "A&amp;B") {
		t.Fatalf("expected escaped name, got %q", output)
	}
	if !strings.Contains(output, "1,234") || !strings.Contains(output, "91,011") {
		t.Fatalf("expected formatted counts, got %q", output)
	}
}
