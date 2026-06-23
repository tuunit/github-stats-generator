package render

import (
	"embed"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tuunit/github-stats-generator/internal/stats"
)

//go:embed templates/*.svg
var templateFS embed.FS

func WriteOutput(dir string, summary stats.Summary) error {
	log.Printf("Rendering SVG brag cards into %q.", dir)

	overviewTemplate, err := templateFS.ReadFile("templates/overview.svg")
	if err != nil {
		return err
	}
	languagesTemplate, err := templateFS.ReadFile("templates/languages.svg")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "overview.svg"), []byte(renderOverview(string(overviewTemplate), summary)), 0o644); err != nil {
		return err
	}
	log.Printf("Rendered overview.svg with %d aggregate repo(s).", summary.RepositoryCount)
	if err := os.WriteFile(filepath.Join(dir, "languages.svg"), []byte(renderLanguages(string(languagesTemplate), summary)), 0o644); err != nil {
		return err
	}
	log.Printf("Rendered languages.svg with %d language slice(s).", len(summary.Languages))
	return nil
}

func renderOverview(template string, summary stats.Summary) string {
	replacer := strings.NewReplacer(
		"{{ name }}", html.EscapeString(summary.Name),
		"{{ stars }}", formatNumber(summary.Stars),
		"{{ forks }}", formatNumber(summary.Forks),
		"{{ contributions }}", formatNumber(summary.Contributions),
		"{{ lines_changed }}", formatNumber(summary.LinesChanged),
		"{{ views }}", formatNumber(summary.Views),
		"{{ repos }}", formatNumber(summary.RepositoryCount),
	)
	return replacer.Replace(template)
}

func renderLanguages(template string, summary stats.Summary) string {
	var progress strings.Builder
	var langList strings.Builder

	for _, language := range summary.Languages {
		color := language.Color
		if color == "" {
			color = "#000000"
		}
		progress.WriteString(fmt.Sprintf(
			`<span style="background-color: %s;width: %.3f%%;" class="progress-item"></span>`,
			html.EscapeString(color),
			language.Percent,
		))
		langList.WriteString(fmt.Sprintf(`
<li>
<svg xmlns="http://www.w3.org/2000/svg" class="octicon" style="fill:%s;" viewBox="0 0 16 16" version="1.1" width="16" height="16"><path fill-rule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8z"></path></svg>
<span class="lang">%s</span>
<span class="percent">%.2f%%</span>
</li>
`, html.EscapeString(color), html.EscapeString(language.Name), language.Percent))
	}

	replacer := strings.NewReplacer(
		"{{ progress }}", progress.String(),
		"{{ lang_list }}", langList.String(),
	)
	return replacer.Replace(template)
}

func formatNumber(value int) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	text := fmt.Sprintf("%d", value)
	if len(text) <= 3 {
		return sign + text
	}

	var out strings.Builder
	remainder := len(text) % 3
	if remainder == 0 {
		remainder = 3
	}
	out.WriteString(text[:remainder])
	for i := remainder; i < len(text); i += 3 {
		out.WriteByte(',')
		out.WriteString(text[i : i+3])
	}
	return sign + out.String()
}
