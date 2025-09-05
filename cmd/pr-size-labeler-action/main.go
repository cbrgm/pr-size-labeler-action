package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
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
	GitHubEnterpriseUrl string `arg:"env:GITHUB_ENTERPRISE_URL"`
}

// Version returns a formatted string with application version details.
func (EnvArgs) Version() string {
	return fmt.Sprintf("Version: %s %s\nBuildTime: %s\n%s\n", Revision, Version, StartTime.Format("2006-01-02"), GoVersion)
}

// Constants for default configuration and event names.
const (
	DefaultConfigPath = ".github/pull-request-size.yml"
	ParamNameFiles    = "files"
	ParamNameDiff     = "diff"
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

// GitHubClientWrapper wraps the GitHub client for ease of testing and abstraction.
type GitHubClientWrapper struct {
	client *github.Client
}

// NewGitHubClientWrapper creates a new wrapper for the GitHub client.
func NewGitHubClientWrapper(token, gitHubEnterpriseUrl string) *GitHubClientWrapper {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Configure GitHub Enterprise URL if provided
	if gitHubEnterpriseUrl != "" {
		var err error
		client, err = client.WithEnterpriseURLs(gitHubEnterpriseUrl, gitHubEnterpriseUrl)
		if err != nil {
			fmt.Printf("Failed to set GitHub Enterprise URLs: %v\n", err)
			// Continue with regular GitHub client if enterprise URL setup fails
		}
	}

	return &GitHubClientWrapper{client: client}
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
func (prp *PullRequestProcessor) ProcessPullRequest() {
	files, err := prp.fetchPullRequestFiles()
	if err != nil {
		exitOnError("fetching pull request files", err)
		return
	}

	numberOfFiles, numberOfLines := calculateSizeAndDiff(files, prp.config)
	size, diff := mapNumberOfChangesToSize(numberOfFiles, numberOfLines, prp.config)
	biggestEntry := getBiggestEntry(prp.config.LabelConfigs, size, diff)

	err = prp.updatePullRequestLabel(biggestEntry)
	if err != nil {
		exitOnError("updating pull request label", err)
	}
}

func main() {
	var args EnvArgs
	arg.MustParse(&args)

	if !isValidGitHubEventType(args.EventName) || !isValidRepoFormat(args.RepoName) {
		return
	}

	prNumber, err := strconv.Atoi(args.PrNumber)
	if err != nil {
		exitOnError("parsing pull request number", err)
		return
	}

	config, err := loadConfig(getConfigFilePath(args.ConfigFilePath))
	if err != nil {
		exitOnError("loading configuration", err)
		return
	}

	ctx := context.Background()
	clientWrapper := NewGitHubClientWrapper(args.GithubToken, args.GitHubEnterpriseUrl)
	prProcessor := NewPullRequestProcessor(ctx, clientWrapper, parseRepoOwner(args.RepoName), parseRepoName(args.RepoName), prNumber, config)
	prProcessor.ProcessPullRequest()
}

// isValidGitHubEventType checks if the event name is a valid pull request event.
func isValidGitHubEventType(eventName string) bool {
	allowedEvents := map[string]bool{
		"pull_request":        true,
		"pull_request_target": true,
	}

	if allowedEvents[strings.ToLower(eventName)] {
		return true
	}

	fmt.Println("Event is not a valid pull request event, doing nothing")
	return false
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

// loadConfig loads the configuration from the YAML file.
func loadConfig(filePath string) (Config, error) {
	var config Config
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(yamlFile, &config)
	return config, err
}

// fetchPullRequestFiles fetches the list of files in a pull request.
func (prp *PullRequestProcessor) fetchPullRequestFiles() ([]*github.CommitFile, error) {
	files, _, err := prp.clientWrapper.client.PullRequests.ListFiles(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber, nil)
	return files, err
}

// updatePullRequestLabel updates the labels of the pull request based on its size.
func (prp *PullRequestProcessor) updatePullRequestLabel(entry ConfigEntry) error {
	pr, _, err := prp.clientWrapper.client.PullRequests.Get(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber)
	if err != nil {
		return err
	}

	err = removeOtherSizeLabels(prp.ctx, prp.clientWrapper.client, prp.repoOwner, prp.repoName, prp.prNumber, pr, prp.config, entry)
	if err != nil {
		return err
	}

	for _, label := range entry.Labels {
		labelExists := labelExists(pr, label)
		if !labelExists {
			_, _, err = prp.clientWrapper.client.Issues.AddLabelsToIssue(prp.ctx, prp.repoOwner, prp.repoName, prp.prNumber, []string{label})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// removeOtherSizeLabels removes labels that are different from the current size labels.
func removeOtherSizeLabels(ctx context.Context, client *github.Client, repoOwner, repoName string, prNumber int, pr *github.PullRequest, config Config, entry ConfigEntry) error {
	for _, label := range pr.Labels {
		if isSizeLabel(label.GetName(), config.LabelConfigs) && !contains(entry.Labels, label.GetName()) {
			_, err := client.Issues.RemoveLabelForIssue(ctx, repoOwner, repoName, prNumber, label.GetName())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// isSizeLabel checks if a label is a size label.
func isSizeLabel(labelName string, labelConfigs []ConfigEntry) bool {
	for _, configLabel := range labelConfigs {
		if contains(configLabel.Labels, labelName) {
			return true
		}
	}
	return false
}

// labelExists checks if a label already exists on a pull request.
func labelExists(pr *github.PullRequest, labelName string) bool {
	for _, label := range pr.Labels {
		if label.GetName() == labelName {
			return true
		}
	}
	return false
}

// calculateSizeAndDiff calculates the size and diff for the pull request.
func calculateSizeAndDiff(files []*github.CommitFile, config Config) (int, int) {
	numberOfFiles, numberOfLines := 0, 0
	for _, file := range files {
		if config.AddedLinesOnly && file.GetStatus() == "removed" {
			continue
		}

		if !shouldExcludeFile(file.GetFilename(), config.ExcludeFiles) {
			numberOfFiles++

			if config.AddedLinesOnly {
				numberOfLines += file.GetAdditions()
			} else {
				numberOfLines += file.GetChanges()
			}
		}
	}
	return numberOfFiles, numberOfLines
}

func mapNumberOfChangesToSize(numberOfFiles, numberOfLines int, config Config) (ConfigEntry, ConfigEntry) {
	size := getSize(config.LabelConfigs, numberOfFiles, ParamNameFiles)
	diff := getSize(config.LabelConfigs, numberOfLines, ParamNameDiff)
	return size, diff
}

// shouldExcludeFile checks if a file should be excluded based on the configuration.
func shouldExcludeFile(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		// Check against the full path
		matched, err := filepath.Match(pattern, filename)
		if err != nil {
			fmt.Printf("Invalid pattern %s: %s\n", pattern, err)
			continue
		}
		if matched {
			return true
		}

		// Extract just the file name and check the pattern again
		justFileName := filepath.Base(filename)
		matched, err = filepath.Match(pattern, justFileName)
		if err != nil {
			fmt.Printf("Invalid pattern %s: %s\n", pattern, err)
			continue
		}
		if matched {
			return true
		}

		// Check if the pattern specifies a directory and matches the beginning of the filename
		if strings.HasSuffix(pattern, "/*") {
			dirPattern := filepath.Dir(pattern)
			if strings.HasPrefix(filename, dirPattern) {
				return true
			}
		}
	}
	return false
}

// getSize retrieves the size configuration based on the number of files or diffs.
func getSize(configuration []ConfigEntry, currentCount int, paramName string) ConfigEntry {
	for _, entry := range configuration {
		var entryValue int
		switch paramName {
		case ParamNameFiles:
			entryValue = entry.Files
		case ParamNameDiff:
			entryValue = entry.Diff
		}

		if currentCount <= entryValue {
			return entry
		}
	}
	return configuration[len(configuration)-1]
}

// getBiggestEntry determines the largest entry between two ConfigEntry objects based on the user-defined order.
func getBiggestEntry(configEntries []ConfigEntry, size, diff ConfigEntry) ConfigEntry {
	sizeIndex := findConfigEntryIndex(configEntries, size.Size)
	diffIndex := findConfigEntryIndex(configEntries, diff.Size)

	if sizeIndex >= diffIndex {
		return size
	}
	return diff
}

// findConfigEntryIndex finds the index of a ConfigEntry in the configuration based on size.
func findConfigEntryIndex(entries []ConfigEntry, size string) int {
	for i, entry := range entries {
		if entry.Size == size {
			return i
		}
	}
	return -1
}

// parseRepoOwner extracts the repository owner from the full repository name.
func parseRepoOwner(repoName string) string {
	parts := strings.Split(repoName, "/")
	return parts[0]
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

// exitOnError terminates the program if an error is encountered.
func exitOnError(action string, err error) {
	if err != nil {
		fmt.Printf("Error %s: %v\n", action, err)
		os.Exit(1)
	}
}

// contains checks if a slice of strings contains a given string.
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
