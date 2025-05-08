package main

import (
	"testing"

	"github.com/google/go-github/v72/github"
)

func TestCalculateSizeAndDiff(t *testing.T) {
	// Define the configuration separately for clarity
	xsConfig := ConfigEntry{"xs", 10, 1, []string{"size/xs"}}
	sConfig := ConfigEntry{"s", 50, 10, []string{"size/s"}}
	mConfig := ConfigEntry{"m", 100, 20, []string{"size/m"}}
	lConfig := ConfigEntry{"l", 500, 50, []string{"size/l"}}
	xlConfig := ConfigEntry{"xl", 1000, 100, []string{"size/xl"}}

	config := Config{
		ExcludeFiles: []string{"exclude.*"},
		LabelConfigs: []ConfigEntry{xsConfig, sConfig, mConfig, lConfig, xlConfig},
	}

	tests := []struct {
		name     string
		files    []*github.CommitFile
		config   Config
		wantSize ConfigEntry
		wantDiff ConfigEntry
	}{
		{
			"No files should result in the smallest size and diff",
			[]*github.CommitFile{},
			config,
			xsConfig,
			xsConfig,
		},
		{
			"Files within 'small' thresholds",
			[]*github.CommitFile{
				mockCommitFile("file1.go", 5),
				mockCommitFile("file2.go", 10),
			},
			config,
			sConfig,
			sConfig,
		},
		{
			"Files exceeding 'small' but within 'medium' thresholds",
			[]*github.CommitFile{
				mockCommitFile("file1.go", 30),
				mockCommitFile("file2.go", 70),
			},
			config,
			sConfig,
			mConfig,
		},
		{
			"Files with one excluded file",
			[]*github.CommitFile{
				mockCommitFile("exclude.txt", 100),
				mockCommitFile("file2.go", 20),
			},
			config,
			xsConfig,
			sConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSize, gotDiff := calculateSizeAndDiff(tt.files, tt.config)
			if !configEntriesAreEqual(gotSize, tt.wantSize) || !configEntriesAreEqual(gotDiff, tt.wantDiff) {
				t.Errorf("calculateSizeAndDiff() = size: %v, diff: %v, want size: %v, want diff: %v", gotSize, gotDiff, tt.wantSize, tt.wantDiff)
			}
		})
	}
}

func TestIsValidGitHubEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		{"Valid Event pull_request", "pull_request", true},
		{"Valid Event pull_request_target", "pull_request_target", true},
		{"Invalid Event empty", "", false},
		{"Invalid Event random string", "random_event", false},
		{"Invalid Event issue", "issue", false},
		{"Invalid Event commit", "commit", false},
		{"Invalid Event push", "push", false},
		{"Invalid Event merge", "merge", false},
		{"Invalid Event null", "null", false},
		{"Invalid Event pull_request_closed", "pull_request_closed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidGitHubEventType(tt.eventName); got != tt.want {
				t.Errorf("isValidGitHubEventType(%v) = %v, want %v", tt.eventName, got, tt.want)
			}
		})
	}
}

func TestGetSize(t *testing.T) {
	// Define configuration entries for clarity
	xsConfig := ConfigEntry{"xs", 10, 1, []string{"size/xs"}}
	sConfig := ConfigEntry{"s", 50, 10, []string{"size/s"}}
	mConfig := ConfigEntry{"m", 100, 20, []string{"size/m"}}
	lConfig := ConfigEntry{"l", 500, 50, []string{"size/l"}}
	xlConfig := ConfigEntry{"xl", 1000, 100, []string{"size/xl"}}

	configuration := []ConfigEntry{xsConfig, sConfig, mConfig, lConfig, xlConfig}

	tests := []struct {
		name          string
		configuration []ConfigEntry
		currentCount  int
		paramName     string
		want          ConfigEntry
	}{
		// Tests for file count
		{"Fewer files than XS threshold", configuration, 0, ParamNameFiles, xsConfig},
		{"Files equal to S threshold", configuration, 10, ParamNameFiles, sConfig},
		{"Files between S and M thresholds", configuration, 15, ParamNameFiles, mConfig},
		{"More files than XL threshold", configuration, 105, ParamNameFiles, xlConfig},

		// Tests for diff count
		{"Fewer changes than XS threshold", configuration, 5, ParamNameDiff, xsConfig},
		{"Changes equal to M threshold", configuration, 100, ParamNameDiff, mConfig},
		{"Changes between S and M thresholds", configuration, 35, ParamNameDiff, sConfig},
		{"More changes than XL threshold", configuration, 1500, ParamNameDiff, xlConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSize(tt.configuration, tt.currentCount, tt.paramName)
			if !configEntriesAreEqual(got, tt.want) {
				t.Errorf("getSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBiggestEntry(t *testing.T) {
	entries := []ConfigEntry{
		{"small", 10, 1, []string{"label1"}},
		{"medium", 20, 2, []string{"label2"}},
		{"large", 30, 3, []string{"label3"}},
	}

	tests := []struct {
		name    string
		entries []ConfigEntry
		size    ConfigEntry
		diff    ConfigEntry
		want    ConfigEntry
	}{
		{"SizeLarger", entries, entries[2], entries[1], entries[2]},
		{"DiffLarger", entries, entries[0], entries[2], entries[2]},
		{"EqualSizeDiff", entries, entries[1], entries[1], entries[1]},
		{"SizeNotInConfig", entries, ConfigEntry{"xlarge", 40, 4, []string{"label4"}}, entries[1], entries[1]},
		{"DiffNotInConfig", entries, entries[1], ConfigEntry{"xlarge", 40, 4, []string{"label4"}}, entries[1]},
		{
			"BothNotInConfig", entries,
			ConfigEntry{"xlarge", 40, 4, []string{"label4"}},
			ConfigEntry{"xxlarge", 50, 5, []string{"label5"}},
			ConfigEntry{"xlarge", 40, 4, []string{"label4"}},
		}, // Expecting `size` to be returned
		{"SingleEntryConfig", []ConfigEntry{{"single", 10, 1, []string{"label1"}}}, ConfigEntry{"single", 10, 1, []string{"label1"}}, ConfigEntry{"single", 10, 1, []string{"label1"}}, ConfigEntry{"single", 10, 1, []string{"label1"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBiggestEntry(tt.entries, tt.size, tt.diff)
			if !configEntriesAreEqual(got, tt.want) {
				t.Errorf("getBiggestEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindConfigEntryIndex(t *testing.T) {
	entries := []ConfigEntry{
		{"small", 10, 1, []string{"label1"}},
		{"medium", 20, 2, []string{"label2"}},
		{"large", 30, 3, []string{"label3"}},
	}

	tests := []struct {
		name    string
		entries []ConfigEntry
		size    string
		want    int
	}{
		{"SizeAtBeginning", entries, "small", 0},
		{"SizeAtEnd", entries, "large", 2},
		{"SizeInMiddle", entries, "medium", 1},
		{"SizeNotExists", entries, "extra-large", -1},
		{"SingleEntryMatch", []ConfigEntry{{"single", 10, 1, []string{"label1"}}}, "single", 0},
		{"SingleEntryNoMatch", []ConfigEntry{{"single", 10, 1, []string{"label1"}}}, "double", -1},
		{"EmptyList", []ConfigEntry{}, "any", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findConfigEntryIndex(tt.entries, tt.size); got != tt.want {
				t.Errorf("findConfigEntryIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRepoOwner(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     string
	}{
		{"ValidRepo", "owner/repo", "owner"},
		{"EmptyOwner", "/repo", ""},
		{"NoSlash", "owner", "owner"},
		{"ExtraSlash", "owner/repo/extra", "owner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRepoOwner(tt.repoName); got != tt.want {
				t.Errorf("parseRepoOwner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     string
	}{
		{"ValidRepo", "owner/repo", "repo"},
		{"EmptyRepo", "owner/", ""},
		{"NoSlash", "repo", "repo"},
		{"ExtraSlash", "owner/repo/extra", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRepoName(tt.repoName); got != tt.want {
				t.Errorf("parseRepoName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidRepoNameFormat(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     bool
	}{
		{"ValidFormat", "owner/repo", true},
		{"NoOwner", "/repo", false},
		{"NoRepo", "owner/", false},
		{"NoSlash", "owner", false},
		{"ExtraSlash", "owner/repo/extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRepoNameFormat(tt.repoName); got != tt.want {
				t.Errorf("isValidRepoNameFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{"Present", []string{"a", "b", "c"}, "b", true},
		{"NotPresent", []string{"a", "b", "c"}, "d", false},
		{"EmptySlice", []string{}, "a", false},
		{"EmptyString", []string{"a", "b", ""}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.slice, tt.item); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func configEntriesAreEqual(a, b ConfigEntry) bool {
	if a.Size != b.Size || a.Diff != b.Diff || a.Files != b.Files {
		return false
	}
	if len(a.Labels) != len(b.Labels) {
		return false
	}
	for i, label := range a.Labels {
		if label != b.Labels[i] {
			return false
		}
	}
	return true
}

func mockCommitFile(filename string, changes int) *github.CommitFile {
	return &github.CommitFile{
		Filename: &filename,
		Changes:  &changes,
	}
}

func TestIsSizeLabel(t *testing.T) {
	labelConfigs := []ConfigEntry{
		{"xs", 10, 1, []string{"size/xs", "review-wanted"}},
		{"s", 50, 10, []string{"size/s", "review-wanted"}},
		// Add more ConfigEntry if needed
	}

	tests := []struct {
		labelName string
		want      bool
	}{
		{"size/xs", true},
		{"review-wanted", true},
		{"size/m", false},
		{"nonexistent-label", false},
	}

	for _, tt := range tests {
		t.Run(tt.labelName, func(t *testing.T) {
			if got := isSizeLabel(tt.labelName, labelConfigs); got != tt.want {
				t.Errorf("isSizeLabel(%v) = %v, want %v", tt.labelName, got, tt.want)
			}
		})
	}
}

func mockPullRequest(labels ...string) *github.PullRequest {
	var githubLabels []*github.Label
	for _, l := range labels {
		label := l // Create a new variable to hold the label name
		githubLabels = append(githubLabels, &github.Label{Name: &label})
	}

	return &github.PullRequest{Labels: githubLabels}
}

func TestLabelExists(t *testing.T) {
	tests := []struct {
		name      string
		pr        *github.PullRequest
		labelName string
		want      bool
	}{
		{"LabelExists", mockPullRequest("size/xs", "bug"), "size/xs", true},
		{"LabelDoesNotExist", mockPullRequest("size/xs", "bug"), "enhancement", false},
		{"EmptyLabels", mockPullRequest(), "size/xs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelExists(tt.pr, tt.labelName); got != tt.want {
				t.Errorf("labelExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		patterns   []string
		wantResult bool
	}{
		{
			name:       "exclude specific file",
			filename:   "foo.bar",
			patterns:   []string{"foo.bar"},
			wantResult: true,
		},
		{
			name:       "exclude by extension",
			filename:   "example.xyz",
			patterns:   []string{"*.xyz"},
			wantResult: true,
		},
		{
			name:       "do not exclude unrelated file",
			filename:   "test.txt",
			patterns:   []string{"*.xyz"},
			wantResult: false,
		},
		{
			name:       "exclude with full path",
			filename:   "/path/to/file/foo.bar",
			patterns:   []string{"/path/to/file/foo.bar"},
			wantResult: true,
		},
		{
			name:       "exclude with wildcard in path",
			filename:   "/some/path/example.txt",
			patterns:   []string{"/some/path/*"},
			wantResult: true,
		},
		{
			name:       "do not exclude when path pattern does not match",
			filename:   "/another/path/example.txt",
			patterns:   []string{"/some/path/*"},
			wantResult: false,
		},
		{
			name:       "exclude with complex path pattern",
			filename:   "/complex/path/with/multiple/sections/file.xyz",
			patterns:   []string{"/complex/path/*/multiple/*/file.xyz"},
			wantResult: true,
		},
		{
			name:       "exclude nested directory file",
			filename:   "/nested/directory/structure/file.txt",
			patterns:   []string{"/nested/directory/*"},
			wantResult: true,
		},
		{
			name:       "exclude using multiple patterns",
			filename:   "multiple.patterns.match",
			patterns:   []string{"*.patterns.*", "multiple.*"},
			wantResult: true,
		},
		{
			name:       "do not exclude when multiple patterns do not match",
			filename:   "no.pattern.match",
			patterns:   []string{"*.patterns.*", "multiple.*"},
			wantResult: false,
		},
		{
			name:       "exclude using complex wildcard patterns",
			filename:   "/var/log/app.log",
			patterns:   []string{"/var/*/app.*", "*.log"},
			wantResult: true,
		},
		{
			name:       "exclude file in root directory",
			filename:   "/rootfile.txt",
			patterns:   []string{"/*.txt"},
			wantResult: true,
		},
		{
			name:       "do not exclude file in non-root directory with root pattern",
			filename:   "/dir/rootfile.txt",
			patterns:   []string{"/*.txt"},
			wantResult: false,
		},
		{
			name:       "exclude using pattern with escaped special characters",
			filename:   "file_with_underscores_and_numbers_123.txt",
			patterns:   []string{"file_with_underscores_and_numbers_*.txt"},
			wantResult: true,
		},
		{
			name:       "exclude using pattern with question mark",
			filename:   "file?.txt",
			patterns:   []string{"file?.txt"},
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldExcludeFile(tt.filename, tt.patterns)
			if result != tt.wantResult {
				t.Errorf("shouldExcludeFile(%v, %v) = %v, want %v", tt.filename, tt.patterns, result, tt.wantResult)
			}
		})
	}
}
