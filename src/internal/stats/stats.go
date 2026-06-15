package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/tuunit/github-stats-generator/internal/githubapi"
)

type RepositorySource string

const (
	RepositoryOwned               RepositorySource = "owned"
	RepositoryCollaborator        RepositorySource = "collaborator"
	RepositoryExternalContributor RepositorySource = "external_contributor"
)

type Language struct {
	Name  string
	Size  int
	Color string
}

type Repository struct {
	Name             string
	OwnerLogin       string
	ViewerPermission string
	Private          bool
	Stars            int
	Forks            int
	Views            int
	LinesChanged     int
	Languages        []Language
	Source           RepositorySource
	Excluded         bool
	Included         bool
}

type CollectedStats struct {
	Login              string
	Name               string
	TotalContributions int
	Emails             []string
	Repositories       []Repository
}

type SummaryLanguage struct {
	Name    string
	Color   string
	Percent float64
	Size    int
}

type Summary struct {
	Name            string
	Stars           int
	Forks           int
	Contributions   int
	LinesChanged    int
	Views           int
	RepositoryCount int
	Languages       []SummaryLanguage
}

type AggregateOptions struct {
	ExcludeRepoPatterns         []string
	ExcludeLanguagePatterns     []string
	ExcludePrivate              bool
	IncludeExternalRepositories bool
}

type Service struct {
	client          *githubapi.Client
	maxStatsRetries int
}

func NewService(client *githubapi.Client, maxStatsRetries int) *Service {
	return &Service{
		client:          client,
		maxStatsRetries: maxStatsRetries,
	}
}

func Aggregate(collected *CollectedStats, options AggregateOptions) Summary {
	summary := Summary{
		Name:          collected.Name,
		Contributions: collected.TotalContributions,
	}

	type languageTotal struct {
		size  int
		color string
	}

	languages := map[string]languageTotal{}
	totalLanguageSize := 0
	excludedCount := 0
	externalSkippedCount := 0

	for index := range collected.Repositories {
		repository := &collected.Repositories[index]
		repository.Excluded = repositoryExcluded(*repository, options)
		if repository.Excluded {
			excludedCount++
			continue
		}

		repository.Included = repository.Source == RepositoryOwned ||
			repository.Source == RepositoryCollaborator ||
			options.IncludeExternalRepositories
		if !repository.Included {
			externalSkippedCount++
			continue
		}

		summary.Stars += repository.Stars
		summary.Forks += repository.Forks
		summary.LinesChanged += repository.LinesChanged
		summary.Views += repository.Views
		summary.RepositoryCount++

		for _, language := range repository.Languages {
			if matchesAnyFold(options.ExcludeLanguagePatterns, language.Name) {
				continue
			}

			current := languages[language.Name]
			current.size += language.Size
			if current.color == "" {
				current.color = language.Color
			}
			languages[language.Name] = current
			totalLanguageSize += language.Size
		}
	}

	summary.Languages = make([]SummaryLanguage, 0, len(languages))
	for name, total := range languages {
		percent := 0.0
		if totalLanguageSize > 0 {
			percent = 100 * float64(total.size) / float64(totalLanguageSize)
		}
		summary.Languages = append(summary.Languages, SummaryLanguage{
			Name:    name,
			Color:   total.color,
			Percent: percent,
			Size:    total.size,
		})
	}

	sort.Slice(summary.Languages, func(i, j int) bool {
		if summary.Languages[i].Size == summary.Languages[j].Size {
			return summary.Languages[i].Name < summary.Languages[j].Name
		}
		return summary.Languages[i].Size > summary.Languages[j].Size
	})

	log.Printf(
		"Aggregate rules kept %d repo(s), skipped %d by filters, and left %d external contribution repo(s) on the bench.",
		summary.RepositoryCount,
		excludedCount,
		externalSkippedCount,
	)
	if len(summary.Languages) > 0 {
		log.Printf("Language podium assembled with %d entry(ies). Top slot: %s.", len(summary.Languages), summary.Languages[0].Name)
	} else {
		log.Printf("Language podium assembled with 0 entries. A minimalist masterpiece.")
	}

	return summary
}

func (s *Service) Collect(ctx context.Context) (*CollectedStats, error) {
	log.Printf("Knocking on GitHub's door for viewer basics.")
	viewer, err := s.fetchViewer(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("Viewer confirmed: %s (%s), contribution history spans %d year(s).", viewer.Name, viewer.Login, len(viewer.Years))

	log.Printf("Collecting commit attribution emails.")
	emails, err := s.fetchEmails(ctx, viewer.Login)
	if err != nil {
		return nil, err
	}
	log.Printf("Email roll call complete: %d address(es) available for line-count attribution.", len(emails))

	log.Printf("Counting all-time contributions across %d year bucket(s).", len(viewer.Years))
	totalContributions, err := s.fetchTotalContributions(ctx, viewer.Years)
	if err != nil {
		return nil, err
	}
	log.Printf("Contribution odometer now reads %d.", totalContributions)

	repositories := map[string]Repository{}

	log.Printf("Fetching owned repositories.")
	owned, err := s.fetchOwnedRepositories(ctx, viewer.Login)
	if err != nil {
		return nil, err
	}
	log.Printf("Owned repository sweep brought back %d repo(s).", len(owned))
	for _, repository := range owned {
		repositories[repository.Name] = repository
	}

	log.Printf("Fetching collaborator and maintainer repositories.")
	collaborator, err := s.fetchCollaboratorRepositories(ctx, viewer.Login)
	if err != nil {
		return nil, err
	}
	log.Printf("Collaborator sweep brought back %d repo(s).", len(collaborator))
	for _, repository := range collaborator {
		repositories[repository.Name] = preferRepository(repositories[repository.Name], repository)
	}

	log.Printf("Fetching external contribution repositories for classification purposes.")
	contributed, err := s.fetchContributedRepositories(ctx, viewer.Login)
	if err != nil {
		return nil, err
	}
	log.Printf("External contribution sweep brought back %d repo(s).", len(contributed))
	for _, repository := range contributed {
		repositories[repository.Name] = preferRepository(repositories[repository.Name], repository)
	}

	result := &CollectedStats{
		Login:              viewer.Login,
		Name:               viewer.Name,
		TotalContributions: totalContributions,
		Emails:             emails,
		Repositories:       make([]Repository, 0, len(repositories)),
	}
	logRepositoryMix(repositories)

	index := 0
	for _, repository := range repositories {
		index++
		log.Printf(
			"[%d/%d] Inspecting %s (%s): %d star(s), %d fork(s), %d language slice(s).",
			index,
			len(repositories),
			repository.Name,
			repository.Source,
			repository.Stars,
			repository.Forks,
			len(repository.Languages),
		)
		if err := s.enrichRepository(ctx, result.Login, emails, &repository); err != nil {
			return nil, err
		}
		log.Printf(
			"[%d/%d] Finished %s: %d views, %d lines changed.",
			index,
			len(repositories),
			repository.Name,
			repository.Views,
			repository.LinesChanged,
		)
		result.Repositories = append(result.Repositories, repository)
	}

	sort.Slice(result.Repositories, func(i, j int) bool {
		return result.Repositories[i].Name < result.Repositories[j].Name
	})
	log.Printf("Repository detail pass complete. %d repo(s) are ready for aggregation.", len(result.Repositories))

	return result, nil
}

func repositoryExcluded(repository Repository, options AggregateOptions) bool {
	if options.ExcludePrivate && repository.Private {
		return true
	}
	return matchesAnyFold(options.ExcludeRepoPatterns, repository.Name)
}

func preferRepository(current, candidate Repository) Repository {
	if current.Name == "" {
		return candidate
	}
	if sourceRank(candidate.Source) > sourceRank(current.Source) {
		return candidate
	}
	if sourceRank(candidate.Source) == sourceRank(current.Source) && candidate.ViewerPermission > current.ViewerPermission {
		return candidate
	}
	return current
}

func sourceRank(source RepositorySource) int {
	switch source {
	case RepositoryOwned:
		return 3
	case RepositoryCollaborator:
		return 2
	case RepositoryExternalContributor:
		return 1
	default:
		return 0
	}
}

func matchesAnyFold(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return false
	}
	lowerValue := strings.ToLower(value)
	for _, pattern := range patterns {
		if ok, _ := path.Match(strings.ToLower(pattern), lowerValue); ok {
			return true
		}
	}
	return false
}

type viewerInfo struct {
	Login string
	Name  string
	Years []int
}

func (s *Service) fetchViewer(ctx context.Context) (viewerInfo, error) {
	const query = `
query {
  viewer {
    login
    name
    contributionsCollection {
      contributionYears
    }
  }
}`

	var response struct {
		Viewer struct {
			Login                   string `json:"login"`
			Name                    string `json:"name"`
			ContributionsCollection struct {
				ContributionYears []int `json:"contributionYears"`
			} `json:"contributionsCollection"`
		} `json:"viewer"`
	}
	if err := s.client.GraphQL(ctx, query, nil, &response); err != nil {
		return viewerInfo{}, err
	}

	name := response.Viewer.Name
	if strings.TrimSpace(name) == "" {
		name = response.Viewer.Login
	}

	return viewerInfo{
		Login: response.Viewer.Login,
		Name:  name,
		Years: response.Viewer.ContributionsCollection.ContributionYears,
	}, nil
}

func (s *Service) fetchEmails(ctx context.Context, login string) ([]string, error) {
	var response []struct {
		Email string `json:"email"`
	}
	status, _, err := s.client.REST(ctx, "/user/emails", &response)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		log.Printf("Email API returned %d, falling back to the noreply address.", status)
		return []string{fmt.Sprintf("%s@users.noreply.github.com", login)}, nil
	}

	emails := make([]string, 0, len(response))
	for _, item := range response {
		if strings.TrimSpace(item.Email) != "" {
			emails = append(emails, item.Email)
		}
	}
	if len(emails) == 0 {
		log.Printf("Email API returned 0 usable addresses, falling back to the noreply address.")
		return []string{fmt.Sprintf("%s@users.noreply.github.com", login)}, nil
	}
	return emails, nil
}

func (s *Service) fetchTotalContributions(ctx context.Context, years []int) (int, error) {
	total := 0
	for _, year := range years {
		query := `
query($from: DateTime!, $to: DateTime!) {
  viewer {
    contributionsCollection(from: $from, to: $to) {
      contributionCalendar {
        totalContributions
      }
    }
  }
}`

		var response struct {
			Viewer struct {
				ContributionsCollection struct {
					ContributionCalendar struct {
						TotalContributions int `json:"totalContributions"`
					} `json:"contributionCalendar"`
				} `json:"contributionsCollection"`
			} `json:"viewer"`
		}

		variables := map[string]any{
			"from": fmt.Sprintf("%04d-01-01T00:00:00Z", year),
			"to":   fmt.Sprintf("%04d-01-01T00:00:00Z", year+1),
		}
		if err := s.client.GraphQL(ctx, query, variables, &response); err != nil {
			return 0, err
		}
		log.Printf("Year %d contributed %d total event(s).", year, response.Viewer.ContributionsCollection.ContributionCalendar.TotalContributions)
		total += response.Viewer.ContributionsCollection.ContributionCalendar.TotalContributions
	}
	return total, nil
}

type graphRepositoryConnection struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []graphRepositoryNode `json:"nodes"`
}

type graphRepositoryNode struct {
	NameWithOwner    string `json:"nameWithOwner"`
	IsPrivate        bool   `json:"isPrivate"`
	ViewerPermission string `json:"viewerPermission"`
	StargazerCount   int    `json:"stargazerCount"`
	ForkCount        int    `json:"forkCount"`
	Owner            struct {
		Login string `json:"login"`
	} `json:"owner"`
	Languages struct {
		Edges []struct {
			Size int `json:"size"`
			Node struct {
				Name  string `json:"name"`
				Color string `json:"color"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"languages"`
}

func (s *Service) fetchOwnedRepositories(ctx context.Context, viewerLogin string) ([]Repository, error) {
	const query = `
query($after: String) {
  viewer {
    repositories(
      first: 100
      after: $after
      affiliations: [OWNER]
      isFork: false
      orderBy: { field: UPDATED_AT, direction: DESC }
    ) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        nameWithOwner
        isPrivate
        viewerPermission
        stargazerCount
        forkCount
        owner { login }
        languages(first: 100, orderBy: { field: SIZE, direction: DESC }) {
          edges {
            size
            node {
              name
              color
            }
          }
        }
      }
    }
  }
}`

	return s.fetchRepositoryPages(ctx, query, "repositories", viewerLogin)
}

func (s *Service) fetchCollaboratorRepositories(ctx context.Context, viewerLogin string) ([]Repository, error) {
	const query = `
query($after: String) {
  viewer {
    repositories(
      first: 100
      after: $after
      affiliations: [COLLABORATOR, ORGANIZATION_MEMBER]
      isFork: false
      orderBy: { field: UPDATED_AT, direction: DESC }
    ) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        nameWithOwner
        isPrivate
        viewerPermission
        stargazerCount
        forkCount
        owner { login }
        languages(first: 100, orderBy: { field: SIZE, direction: DESC }) {
          edges {
            size
            node {
              name
              color
            }
          }
        }
      }
    }
  }
}`

	repositories, err := s.fetchRepositoryPages(ctx, query, "repositories", viewerLogin)
	if err != nil {
		return nil, err
	}

	filtered := make([]Repository, 0, len(repositories))
	for _, repository := range repositories {
		if repository.Source == RepositoryCollaborator {
			filtered = append(filtered, repository)
		}
	}
	return filtered, nil
}

func (s *Service) fetchContributedRepositories(ctx context.Context, viewerLogin string) ([]Repository, error) {
	const query = `
query($after: String) {
  viewer {
    repositoriesContributedTo(
      first: 100
      after: $after
      includeUserRepositories: false
      contributionTypes: [COMMIT, PULL_REQUEST, REPOSITORY, PULL_REQUEST_REVIEW]
      orderBy: { field: UPDATED_AT, direction: DESC }
    ) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        nameWithOwner
        isPrivate
        viewerPermission
        stargazerCount
        forkCount
        owner { login }
        languages(first: 100, orderBy: { field: SIZE, direction: DESC }) {
          edges {
            size
            node {
              name
              color
            }
          }
        }
      }
    }
  }
}`

	return s.fetchRepositoryPages(ctx, query, "repositoriesContributedTo", viewerLogin)
}

func (s *Service) fetchRepositoryPages(ctx context.Context, query, connectionKey, viewerLogin string) ([]Repository, error) {
	var repositories []Repository
	var after *string

	for {
		variables := map[string]any{}
		if after != nil {
			variables["after"] = *after
		} else {
			variables["after"] = nil
		}

		var response struct {
			Viewer map[string]json.RawMessage `json:"viewer"`
		}
		if err := s.client.GraphQL(ctx, query, variables, &response); err != nil {
			return nil, err
		}

		rawConnection, ok := response.Viewer[connectionKey]
		if !ok {
			return nil, fmt.Errorf("missing %s connection in graphql response", connectionKey)
		}

		var connection graphRepositoryConnection
		if err := json.Unmarshal(rawConnection, &connection); err != nil {
			return nil, err
		}
		log.Printf("Fetched %d repo(s) from %s page.", len(connection.Nodes), connectionKey)

		for _, node := range connection.Nodes {
			repositories = append(repositories, convertRepository(node, viewerLogin))
		}

		if !connection.PageInfo.HasNextPage || connection.PageInfo.EndCursor == "" {
			break
		}
		next := connection.PageInfo.EndCursor
		after = &next
	}

	return repositories, nil
}

func convertRepository(node graphRepositoryNode, viewerLogin string) Repository {
	repository := Repository{
		Name:             node.NameWithOwner,
		OwnerLogin:       node.Owner.Login,
		ViewerPermission: node.ViewerPermission,
		Private:          node.IsPrivate,
		Stars:            node.StargazerCount,
		Forks:            node.ForkCount,
		Source:           classifyRepository(node.Owner.Login, viewerLogin, node.ViewerPermission),
		Languages:        make([]Language, 0, len(node.Languages.Edges)),
	}

	for _, edge := range node.Languages.Edges {
		repository.Languages = append(repository.Languages, Language{
			Name:  edge.Node.Name,
			Size:  edge.Size,
			Color: edge.Node.Color,
		})
	}

	return repository
}

func classifyRepository(ownerLogin, viewerLogin, viewerPermission string) RepositorySource {
	if strings.EqualFold(ownerLogin, viewerLogin) {
		return RepositoryOwned
	}
	if collaboratorPermission(viewerPermission) {
		return RepositoryCollaborator
	}
	return RepositoryExternalContributor
}

func collaboratorPermission(permission string) bool {
	switch permission {
	case "ADMIN", "MAINTAIN", "WRITE", "TRIAGE":
		return true
	default:
		return false
	}
}

func (s *Service) enrichRepository(ctx context.Context, login string, emails []string, repository *Repository) error {
	linesChanged, err := s.fetchLinesChanged(ctx, login, emails, repository.Name)
	if err != nil {
		return err
	}
	repository.LinesChanged = linesChanged

	views, err := s.fetchViews(ctx, repository.Name)
	if err != nil {
		return err
	}
	repository.Views = views
	return nil
}

func (s *Service) fetchViews(ctx context.Context, repository string) (int, error) {
	var response struct {
		Count int `json:"count"`
	}
	status, _, err := s.client.REST(ctx, "/repos/"+repository+"/traffic/views", &response)
	if err != nil {
		return 0, err
	}
	if status != 200 {
		log.Printf("Views for %s came back with HTTP %d, so this round scores 0.", repository, status)
		return 0, nil
	}
	return response.Count, nil
}

func (s *Service) fetchLinesChanged(ctx context.Context, login string, emails []string, repository string) (int, error) {
	for attempt := 0; attempt < s.maxStatsRetries; attempt++ {
		status, body, err := s.client.REST(ctx, "/repos/"+repository+"/stats/contributors", nil)
		if err != nil {
			return 0, err
		}

		switch status {
		case 200:
			total, err := decodeContributorStats(body, login)
			if err != nil {
				return 0, fmt.Errorf("decode contributor stats for %s: %w", repository, err)
			}
			log.Printf("Contributor stats API yielded %d changed line(s) for %s.", total, repository)
			return total, nil
		case 202:
			log.Printf("Contributor stats for %s are still baking (attempt %d/%d). Waiting 2s.", repository, attempt+1, s.maxStatsRetries)
			time.Sleep(2 * time.Second)
			continue
		case 403, 429:
			log.Printf("Contributor stats for %s hit HTTP %d. Falling back to git archaeology.", repository, status)
			return s.gitLinesChanged(ctx, emails, repository)
		default:
			log.Printf("Contributor stats for %s returned HTTP %d. Falling back to git archaeology.", repository, status)
			return s.gitLinesChanged(ctx, emails, repository)
		}
	}

	log.Printf("Contributor stats for %s stayed in the oven too long. Switching to git archaeology.", repository)
	return s.gitLinesChanged(ctx, emails, repository)
}

func (s *Service) gitLinesChanged(ctx context.Context, emails []string, repository string) (int, error) {
	if _, err := exec.LookPath("git"); err != nil {
		log.Printf("git is unavailable, so %s gets 0 fallback lines changed.", repository)
		return 0, nil
	}

	repoDir, err := os.MkdirTemp("", "github-stats-generator-repo-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(repoDir)

	log.Printf("Cloning %s for fallback line counting.", repository)
	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", s.client.Token(), repository)
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--bare", "--filter=blob:none", "--no-tags", "--single-branch", cloneURL, repoDir)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("git clone failed for %s: %s", repository, strings.TrimSpace(string(output)))
	}

	args := []string{"-C", repoDir, "log", "--numstat", "--pretty=tformat:"}
	for _, email := range emails {
		args = append(args, "--author", email)
	}
	logCmd := exec.CommandContext(ctx, "git", args...)
	output, err := logCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return 0, fmt.Errorf("git log failed for %s: %s", repository, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return 0, err
	}

	total := 0
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		additions := parseNumstatValue(fields[0])
		deletions := parseNumstatValue(fields[1])
		total += additions + deletions
	}
	log.Printf("git fallback counted %d changed line(s) for %s.", total, repository)

	return total, nil
}

func decodeContributorStats(body []byte, login string) (int, error) {
	var contributors []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Weeks []struct {
			Additions int `json:"a"`
			Deletions int `json:"d"`
		} `json:"weeks"`
	}
	if err := json.Unmarshal(body, &contributors); err != nil {
		return 0, err
	}

	total := 0
	for _, contributor := range contributors {
		if !strings.EqualFold(contributor.Author.Login, login) {
			continue
		}
		for _, week := range contributor.Weeks {
			total += week.Additions + week.Deletions
		}
	}
	return total, nil
}

func logRepositoryMix(repositories map[string]Repository) {
	owned := 0
	collaborator := 0
	external := 0
	private := 0
	for _, repository := range repositories {
		switch repository.Source {
		case RepositoryOwned:
			owned++
		case RepositoryCollaborator:
			collaborator++
		case RepositoryExternalContributor:
			external++
		}
		if repository.Private {
			private++
		}
	}
	log.Printf(
		"Repository roster assembled: %d total (%d owned, %d collaborator, %d external, %d private).",
		len(repositories),
		owned,
		collaborator,
		external,
		private,
	)
}

func parseNumstatValue(value string) int {
	if value == "-" {
		return 0
	}
	var parsed int
	_, err := fmt.Sscanf(value, "%d", &parsed)
	if err != nil {
		return 0
	}
	return parsed
}
