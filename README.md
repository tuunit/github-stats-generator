# GitHub Stats Generator

Generate `overview.svg` and `languages.svg` from your GitHub activity, either by running the Go CLI yourself or by calling the reusable GitHub Action from another repository such as your profile repository (`<username>/<username>`).

![Languages](examples/languages.svg)
![Overview](examples/overview.svg)

## What it does

- Collects GitHub profile and repository statistics.
- Renders deterministic SVGs for GitHub README embedding.
- Counts stars and forks from owned repositories and explicit collaborator/maintainer repositories by default.
- Lets you write the generated SVGs anywhere with `--output-dir` or `OUTPUT_DIR`.

## Personal access token setup

Create a **classic** personal access token and store it as a secret in the repository where you will run the tool or Action.

1. Open <https://github.com/settings/tokens>.
2. Choose **Generate new token** -> **Generate new token (classic)**.
3. Give it a descriptive name such as `github-stats-generator`.
4. Choose an expiration that fits your preference.
5. Grant these scopes:
   - `read:user`
   - `user:email`
   - `repo`
6. Generate the token and copy it immediately.
7. Add it as a repository secret such as `ACCESS_TOKEN` or `STATS_TOKEN`.

Why those scopes:

- `read:user` is needed for viewer metadata.
- `user:email` is needed for commit attribution and git fallback line counting.
- `repo` is needed to read private repositories and related repository metadata when your token has access.

## Configuration

The CLI supports flags, environment variables, and corresponding Action inputs.

| Purpose | CLI flag | Environment variable(s) | Action input |
| --- | --- | --- | --- |
| GitHub token | `--access-token` | `ACCESS_TOKEN` | `access_token` |
| Output directory | `--output-dir` | `OUTPUT_DIR` | `output_dir` |
| Excluded repositories | `--exclude-repos` | `EXCLUDE_REPOS`, `EXCLUDED` | `exclude_repos` |
| Excluded languages | `--exclude-langs` | `EXCLUDE_LANGS`, `EXCLUDED_LANGS` | `exclude_langs` |
| Exclude private repositories | `--exclude-private` | `EXCLUDE_PRIVATE` | `exclude_private` |
| Include external contribution repositories | `--include-contributed-repositories` | `INCLUDE_CONTRIBUTED_REPOSITORIES` | `include_contributed_repositories` |
| Legacy compatibility toggle for contributed repos | n/a | `EXCLUDE_FORKED_REPOS` | `exclude_forked_repos` |
| Contributor stats retry count | `--max-retries` | `MAX_RETRIES` | `max_retries` |

Notes:

- `EXCLUDE_REPOS` and `EXCLUDE_LANGS` accept comma-separated glob patterns.
- `EXCLUDE_FORKED_REPOS=true` keeps externally contributed repositories excluded, matching the older Python workflow behavior.
- If both `INCLUDE_CONTRIBUTED_REPOSITORIES` and `EXCLUDE_FORKED_REPOS` are set, the direct include flag is the clearer modern control.

## Standalone CLI usage

### Run from source

```bash
cd src
ACCESS_TOKEN=ghp_your_token_here \
go run ./cmd/github-stats-generator --output-dir ../generated
```

That writes:

- `generated/overview.svg`
- `generated/languages.svg`

### Use a released binary

After this repository starts publishing releases, download the archive for your platform from the Releases page, extract it, and run:

```bash
ACCESS_TOKEN=ghp_your_token_here \
./github-stats-generator --output-dir .
```

Using `--output-dir .` writes the SVGs next to your README, which is handy for profile repositories.

## Reusable GitHub Action usage

This repository ships a GitHub Action that downloads the latest released binary by default and runs it inside the caller repository.

Example workflow for a profile repository:

```yaml
name: Update profile stats

on:
  workflow_dispatch:
  schedule:
    - cron: "15 3 * * *"

permissions:
  contents: write

jobs:
  update-stats:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate SVG stats
        uses: tuunit/github-stats-generator@v0.2.2
        with:
          access_token: ${{ secrets.ACCESS_TOKEN }}
          output_dir: .
          exclude_repos: ""
          exclude_langs: ""
          exclude_private: "false"
          include_contributed_repositories: "false"
          max_retries: "15"

      - name: Commit generated SVGs
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git add overview.svg languages.svg
          git commit -m "Update GitHub stats" || true
          git push
```

### Action inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `access_token` | yes | none | GitHub personal access token |
| `output_dir` | no | `.` | Directory where `overview.svg` and `languages.svg` will be written |
| `version` | no | `latest` | Release tag to download, such as `v1.2.3` |
| `release_repository` | no | current action repository | Repository that hosts the release binaries |
| `exclude_repos` | no | empty | Comma-separated repo glob patterns |
| `exclude_langs` | no | empty | Comma-separated language glob patterns |
| `exclude_private` | no | `false` | Exclude private repositories from aggregates |
| `include_contributed_repositories` | no | `false` | Include repositories you only contributed to |
| `exclude_forked_repos` | no | empty | Legacy compatibility toggle |
| `max_retries` | no | `15` | Retry count before falling back to `git log` |

## Embedding the generated SVGs

If the files live next to your profile README, you can reference them directly:

```markdown
![](./overview.svg)
![](./languages.svg)
```

## Aggregation rules

- Stars and forks include owned repositories and repositories where you are an explicit collaborator or maintainer.
- Repositories you only contributed to are excluded by default.
- Set `INCLUDE_CONTRIBUTED_REPOSITORIES=true` to include those external contribution repositories.
- Private repositories follow the same rules when the token can access them.

## Releases

Release binaries are produced with GoReleaser. The reusable Action downloads those release artifacts instead of building the Go binary inside the caller repository.
