# PR Size Labeler GitHub Action

**Automatically labels pull requests in your GitHub repository based on the size of changes.**

[![GitHub release](https://img.shields.io/github/release/cbrgm/pr-size-labeler-action.svg)](https://github.com/cbrgm/pr-size-labeler-action)
[[Docker Repository on Quay](https://quay.io/repository/cbrgm/pr-size-labeler-action/status "Docker Repository on Quay")](https://quay.io/repository/cbrgm/pr-size-labeler-action)

## Workflow Usage

Before using this action, ensure you have a [`.github/pull-request-size.yml`](.github/pull-request-size.yml) configuration file in your repository. This file should define the size thresholds and corresponding labels.

Add the following step to your GitHub Actions Workflow:

```yaml
name: PR Size Labeler

on:
  pull_request:

jobs:
  label-pr:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Repository
        uses: actions/checkout@v2

      - name: Label PR based on size
        uses: cbrgm/pr-size-labeler-action@main
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          github_repository: ${{ github.repository }}
          github_pr_number: ${{ github.event.number }}
          config_file_path: '.github/pull-request-size.yml'
```

### Local Development

You can build this action from source using `Go`:

```bash
make build
```

## Contributing & License

Feel free to submit changes! See the [Contributing Guide](https://github.com/cbrgm/contributing/blob/master/CONTRIBUTING.md). This project is open-source
and is developed under the terms of the [Apache 2.0 License](https://github.com/cbrgm/pr-size-labeler-action/blob/master/LICENSE).
