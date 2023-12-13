# PR Size Labeler GitHub Action

**Automatically labels pull requests in your GitHub repository based on the size of changes.**

[![GitHub release](https://img.shields.io/github/release/cbrgm/pr-size-labeler-action.svg)](https://github.com/cbrgm/pr-size-labeler-action)
[![Go Report Card](https://goreportcard.com/badge/github.com/cbrgm/pr-size-labeler-action)](https://goreportcard.com/report/github.com/cbrgm/pr-size-labeler-action)
[![go-lint-test](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/go-lint-test.yml/badge.svg)](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/go-lint-test.yml)
[![go-binaries](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/go-binaries.yml/badge.svg)](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/go-binaries.yml)
[![container](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/container.yml/badge.svg)](https://github.com/cbrgm/cbrgm-pr-size-labeler-action/actions/workflows/container.yml)

## Workflow Usage

Before using this action, ensure you have a [`.github/pull-request-size.yml`](.github/pull-request-size.yml) configuration file in your repository. This file should define the size thresholds and corresponding labels.

Add the following step to your GitHub Actions Workflow:

```yaml
name: PR Size Labeler

on:
  pull_request: # Trigger the workflow when a pull request is opened or synchronized

jobs:
  auto-label-pr:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Repository
        uses: actions/checkout@v2 # Checkout the repository code
      - name: Label PR based on size
        uses: cbrgm/pr-size-labeler-action@main
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }} # Pass the GitHub token for authentication
          github_repository: ${{ github.repository }} # Pass the repository name
          github_pr_number: ${{ github.event.number }} # Pass the pull request number
          config_file_path: '.github/pull-request-size.yml' # Specify the path to the configuration file
```

## Example Config

```yml
# Configuration for PR Size Labeler

# List of files to exclude from size calculation
# Files matching these patterns will not be considered when calculating PR size
exclude_files:
  - "foo.bar"  # Example: Exclude 'foo.bar' file
  - "*.xyz"

# Configuration for labeling based on the size of the Pull Request
# Each entry defines a size label, along with thresholds for diff and file count
label_configs:
  # Configuration for 'extra small' PRs
  - size: xs
    diff: 25    # Threshold for the total lines of code changed (additions + deletions)
    files: 1    # Threshold for the total number of files changed
    labels: ["size/xs"]  # Labels to be applied for this size

  # Configuration for 'small' PRs
  - size: s
    diff: 150
    files: 10
    labels: ["size/s"]

  # Configuration for 'medium' PRs
  - size: m
    diff: 600
    files: 25
    labels: ["size/m", "pairing-wanted"]

  # Configuration for 'large' PRs
  - size: l
    diff: 2500
    files: 50
    labels: ["size/l", "pairing-wanted"]

  # Configuration for 'extra large' PRs
  - size: xl
    diff: 5000
    files: 100
    labels: ["size/xl", "pairing-wanted"]
```

### Local Development

You can build this action from source using `Go`:

```bash
make build
```

## Contributing & License

Your contributions are highly welcomed and appreciated! If you're interested in enhancing this project, please refer to our detailed [Contributing Guide](https://github.com/cbrgm/pr-size-labeler-action/blob/main/CONTRIBUTING.md) for guidelines and best practices. This guide will assist you in understanding the process for submitting changes, setting up your development environment, and more.

This project is a labor of love, developed in my free time and provided for free to the community. As an open-source initiative, it operates under the [Apache 2.0 License](https://github.com/cbrgm/pr-size-labeler-action/blob/main/LICENSE). This license grants you legal permission to use, modify, and distribute this software. Please review the license terms to understand your rights and responsibilities.

Your involvement, whether through contributions, suggestions, or feedback, is essential for keeping this project vibrant and beneficial to all. Let's collaborate to make this tool even better!
