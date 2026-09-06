package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/google/go-github/v91/github"
	"gopkg.in/yaml.v3"
)

// Global variables for application metadata.
var (
	Version   string              // Version of the application.
	Revision  string              // Revision or Commit this binary was built from.
	GoVersion = runtime.Version() // GoVersion running this binary.
	StartTime = time.Now()        // StartTime of the application.
)

// EnvArgs struct holds the required environment variables.
type EnvArgs struct {
	GithubToken         string `arg:"env:GITHUB_TOKEN,required"`
	EventName           string `arg:"env:GITHUB_EVENT_NAME,required"`
	PrNumber            string `arg:"env:PULL_REQUEST_NUMBER,required"`
	RepoName            string `arg:"env:GITHUB_REPOSITORY,required"`
	ConfigFilePath      string `arg:"env:CONFIG_FILE_PATH"`
	GitHubEnterpriseURL string `arg:"env:GITHUB_ENTERPRISE_URL"`
}

// Version returns a formatted string with application version details.
func (EnvArgs) Version() string {
	return fmt.Sprintf("Version: %s %s\nBuildTime: %s\n%s\n", Revision, Version, StartTime.Format("2006-01-02"), GoVersion)
}

// Constants for default configuration and event names.
const (
	DefaultConfigPath = ".github/pull-request-size.yml"

	// maxFilesPerPage is the largest page size the GitHub API accepts for
	// listing pull request files.
	maxFilesPerPage = 100
)

// ConfigEntry defines a single configuration entry for label assignment.
type ConfigEntry struct {
	Size   string   `yaml:"size"`
	Diff   int      `yaml:"diff"`
	Files  int      `yaml:"files"`
	Labels []string `yaml:"labels"` // Updated to support multiple labels
}

// Config struct holds the entire configuration for label assignment.
type Config struct {
	ExcludeFiles   []string      `yaml:"exclude_files"`
	LabelConfigs   []ConfigEntry `yaml:"label_configs"`
	AddedLinesOnly bool          `yaml:"added_lines_only"`
}

// thresholdFunc reads the threshold a ConfigEntry defines for one of the two
// size axes.
type thresholdFunc func(ConfigEntry) int

// filesThreshold returns the file count threshold of an entry.
func filesThreshold(entry ConfigEntry) int { return entry.Files }

// diffThreshold returns the changed lines threshold of an entry.
func diffThreshold(entry ConfigEntry) int { return entry.Diff }

// GitHubClientWrapper wraps the GitHub client for ease of testing and abstraction.
type GitHubClientWrapper struct {
	client *github.Client
}

// NewGitHubClientWrapper creates a new wrapper for the GitHub client.
func NewGitHubClientWrapper(token, gitHubEnterpriseURL string) (*GitHubClientWrapper, error) {
	opts := []github.ClientOptionsFunc{github.WithAuthToken(token)}
	if gitHubEnterpriseURL != "" {
		opts = append(opts, github.WithEnterpriseURLs(gitHubEnterpriseURL, gitHubEnterpriseURL))
	}

	client, err := github.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &GitHubClientWrapper{client: client}, nil
}

// PullRequestProcessor handles the processing of a single pull request.
type PullRequestProcessor struct {
	clientWrapper *GitHubClientWrapper
	repoOwner     string
	repoName      string
	prNumber      int
	config        Config
	ctx           context.Context
}

// NewPullRequestProcessor creates a new PullRequestProcessor instance.
func NewPullRequestProcessor(ctx context.Context, clientWrapper *GitHubClientWrapper, repoOwner, repoName string, prNumber int, config Config) *PullRequestProcessor {
	return &PullRequestProcessor{
		clientWrapper: clientWrapper,
		repoOwner:     repoOwner,
		repoName:      repoName,
		prNumber:      prNumber,
		config:        config,
		ctx:           ctx,
	}
}

// ProcessPullRequest processes the files of a pull request and applies labels accordingly.
func (prp *PullRequestProcessor) ProcessPullRequest() error {
	files, err := prp.fetchPullRequestFiles()
	if err != nil {
		return fmt.Errorf("fetching pull request files: %w", err)
	}

	numberOfFiles, numberOfLines := calculateSizeAndDiff(files, prp.config)
	size, diff := mapNumberOfChangesToSize(numberOfFiles, numberOfLines, prp.config)
	biggestEntry := getBiggestEntry(prp.config.LabelConfigs, size, diff)

	if err := prp.updatePullRequestLabel(biggestEntry); err != nil {
		return fmt.Errorf("updating pull request label: %w", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run wires everything together and reports the first error it hits.
func run() error {
	var args EnvArgs
	arg.MustParse(&args)

	if !isValidGitHubEventType(args.EventName) || !isValidRepoFormat(args.RepoName) {
		return nil
	}

	prNumber, err := strconv.Atoi(args.PrNumber)
	if err != nil {
		return fmt.Errorf("parsing pull request number %q: %w", args.PrNumber, err)
	}

	config, err := loadConfig(getConfigFilePath(args.ConfigFilePath))
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	clientWrapper, err := NewGitHubClientWrapper(args.GithubToken, args.GitHubEnterpriseURL)
	if err != nil {
		return fmt.Errorf("creating GitHub client: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	processor := NewPullRequestProcessor(ctx, clientWrapper, parseRepoOwner(args.RepoName), parseRepoName(args.RepoName), prNumber, config)
	return processor.ProcessPullRequest()
}

// isValidGitHubEventType checks if the event name is a valid pull request event.
func isValidGitHubEventType(eventName string) bool {
	switch strings.ToLower(eventName) {
	case "pull_request", "pull_request_target":
		return true
	default:
		fmt.Println("Event is not a valid pull request event, doing nothing")
		return false
	}
}

// isValidRepoFormat checks if the repository name follows the 'owner/repository' format.
func isValidRepoFormat(repoName string) bool {
	if !isValidRepoNameFormat(repoName) {
		fmt.Printf("Repository name is in the wrong format. Expected 'owner/repository'\n")
		return false
	}
	return true
}

// getConfigFilePath retrieves the configuration file path or sets a default.
func getConfigFilePath(providedPath string) string {
	if providedPath == "" {
		return DefaultConfigPath
	}
	return providedPath
}

// loadConfig loads and validates the configuration from the YAML file.
func loadConfig(filePath string) (Config, error) {
	var config Config

	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return config, fmt.Errorf("reading %s: %w", filePath, err)
	}

	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return config, fmt.Errorf("parsing %s: %w", filePath, err)
	}

	if err := validateConfig(config); err != nil {
		return config, fmt.Errorf("invalid configuration in %s: %w", filePath, err)
	}

	return config, nil
}

// validateConfig checks that the configuration can actually be used to pick a
// label, so a malformed file fails with a readable message instead of a panic
// further down.
func validateConfig(config Config) error {
	if len(config.LabelConfigs) == 0 {
		return errors.New("label_configs must contain at least one entry")
	}

	for i, entry := range config.LabelConfigs {
		if entry.Size == "" {
			return fmt.Errorf("label_configs[%d]: size must not be empty", i)
		}
		if len(entry.Labels) == 0 {
			return fmt.Errorf("label_configs[%d] (%s): labels must not be empty", i, entry.Size)
		}
	}

	return nil
}

// fetchPullRequestFiles fetches the list of files in a pull request, following
// pagination so that pull requests with more than one page of changed files are
// counted in full.
func (prp *PullRequestProcessor) fetchPullRequestFiles() ([]*github.CommitFile, error) {
	opts := &github.ListOptions{PerPage: maxFilesPerPage}

	var allFiles []*github.CommitFile
	for {
		files, resp, err := prp.clientWrapper.client.PullRequests.ListFiles(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber, opts)
		if err != nil {
			return nil, err
		}

		allFiles = append(allFiles, files...)
		if resp.NextPage == 0 {
			return allFiles, nil
		}
		opts.Page = resp.NextPage
	}
}

// updatePullRequestLabel updates the labels of the pull request based on its size.
func (prp *PullRequestProcessor) updatePullRequestLabel(entry ConfigEntry) error {
	pr, _, err := prp.clientWrapper.client.PullRequests.Get(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber)
	if err != nil {
		return fmt.Errorf("fetching pull request: %w", err)
	}

	if err := prp.removeOtherSizeLabels(pr, entry); err != nil {
		return err
	}

	missing := slices.DeleteFunc(slices.Clone(entry.Labels), func(label string) bool {
		return labelExists(pr, label)
	})
	if len(missing) == 0 {
		return nil
	}

	if _, _, err := prp.clientWrapper.client.Issues.AddLabelsToIssue(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber, missing); err != nil {
		return fmt.Errorf("adding labels %v: %w", missing, err)
	}

	return nil
}

// removeOtherSizeLabels removes labels that are different from the current size labels.
func (prp *PullRequestProcessor) removeOtherSizeLabels(pr *github.PullRequest, entry ConfigEntry) error {
	for _, label := range pr.Labels {
		name := label.GetName()
		if !isSizeLabel(name, prp.config.LabelConfigs) || slices.Contains(entry.Labels, name) {
			continue
		}

		if _, err := prp.clientWrapper.client.Issues.RemoveLabelForIssue(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber, name); err != nil {
			return fmt.Errorf("removing label %q: %w", name, err)
		}
	}

	return nil
}

// isSizeLabel checks if a label is a size label.
func isSizeLabel(labelName string, labelConfigs []ConfigEntry) bool {
	return slices.ContainsFunc(labelConfigs, func(entry ConfigEntry) bool {
		return slices.Contains(entry.Labels, labelName)
	})
}

// labelExists checks if a label already exists on a pull request.
func labelExists(pr *github.PullRequest, labelName string) bool {
	return slices.ContainsFunc(pr.Labels, func(label *github.Label) bool {
		return label.GetName() == labelName
	})
}

// calculateSizeAndDiff calculates the size and diff for the pull request.
func calculateSizeAndDiff(files []*github.CommitFile, config Config) (int, int) {
	numberOfFiles, numberOfLines := 0, 0
	for _, file := range files {
		if config.AddedLinesOnly && file.GetStatus() == "removed" {
			continue
		}

		if shouldExcludeFile(file.GetFilename(), config.ExcludeFiles) {
			continue
		}

		numberOfFiles++
		if config.AddedLinesOnly {
			numberOfLines += file.GetAdditions()
		} else {
			numberOfLines += file.GetChanges()
		}
	}
	return numberOfFiles, numberOfLines
}

func mapNumberOfChangesToSize(numberOfFiles, numberOfLines int, config Config) (ConfigEntry, ConfigEntry) {
	size := getSize(config.LabelConfigs, numberOfFiles, filesThreshold)
	diff := getSize(config.LabelConfigs, numberOfLines, diffThreshold)
	return size, diff
}

// shouldExcludeFile checks if a file should be excluded based on the configuration.
func shouldExcludeFile(filename string, patterns []string) bool {
	justFileName := filepath.Base(filename)

	for _, pattern := range patterns {
		// Check against the full path. filepath.Match only ever fails on a
		// malformed pattern, so warning once per pattern is enough.
		matched, err := filepath.Match(pattern, filename)
		if err != nil {
			fmt.Printf("Invalid pattern %s: %s\n", pattern, err)
			continue
		}
		if matched {
			return true
		}

		// Check the pattern against the file name alone.
		if matched, _ := filepath.Match(pattern, justFileName); matched {
			return true
		}

		// A trailing "/*" excludes the directory recursively, which filepath.Match
		// cannot express because its wildcards never cross a separator.
		if dir, ok := strings.CutSuffix(pattern, "/*"); ok && strings.HasPrefix(filename, dir+"/") {
			return true
		}
	}
	return false
}

// getSize retrieves the size configuration based on the number of files or diffs.
func getSize(configuration []ConfigEntry, currentCount int, threshold thresholdFunc) ConfigEntry {
	i := slices.IndexFunc(configuration, func(entry ConfigEntry) bool {
		return currentCount <= threshold(entry)
	})
	if i >= 0 {
		return configuration[i]
	}
	return configuration[len(configuration)-1]
}

// getBiggestEntry determines the largest entry between two ConfigEntry objects based on the user-defined order.
func getBiggestEntry(configEntries []ConfigEntry, size, diff ConfigEntry) ConfigEntry {
	if findConfigEntryIndex(configEntries, size.Size) >= findConfigEntryIndex(configEntries, diff.Size) {
		return size
	}
	return diff
}

// findConfigEntryIndex finds the index of a ConfigEntry in the configuration based on size.
func findConfigEntryIndex(entries []ConfigEntry, size string) int {
	return slices.IndexFunc(entries, func(entry ConfigEntry) bool {
		return entry.Size == size
	})
}

// parseRepoOwner extracts the repository owner from the full repository name.
func parseRepoOwner(repoName string) string {
	owner, _, _ := strings.Cut(repoName, "/")
	return owner
}

// parseRepoName extracts the repository name from the full repository name.
func parseRepoName(repoName string) string {
	parts := strings.Split(repoName, "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return repoName
}

// isValidRepoNameFormat checks if a given repository name is in the 'owner/repository' format.
func isValidRepoNameFormat(repoName string) bool {
	parts := strings.Split(repoName, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
