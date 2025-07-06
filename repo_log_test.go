package gittools

import (
	"reflect"
	"testing"
)

func TestParseOnelineFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []LogItem
	}{
		{
			name: "simple oneline with tags",
			input: `1d9a112 (HEAD -> main, tag: v0.0.6, origin/main, origin/HEAD) Add ls-files command to repo
14d383b (tag: v0.0.5) Add diff command to repo
151c718 (tag: v0.0.4) Walk up the directory tree to find the git repo's root`,
			expected: []LogItem{
				{
					Commit:  "1d9a112",
					Author:  "",
					Date:    "",
					Message: "Add ls-files command to repo",
					Tags:    []string{"v0.0.6"},
				},
				{
					Commit:  "14d383b",
					Author:  "",
					Date:    "",
					Message: "Add diff command to repo",
					Tags:    []string{"v0.0.5"},
				},
				{
					Commit:  "151c718",
					Author:  "",
					Date:    "",
					Message: "Walk up the directory tree to find the git repo's root",
					Tags:    []string{"v0.0.4"},
				},
			},
		},
		{
			name: "oneline with no tags",
			input: `393a6c6 (refactor-repo-structure) Refactor layout
17f9919 Remove some excess calls`,
			expected: []LogItem{
				{
					Commit:  "393a6c6",
					Author:  "",
					Date:    "",
					Message: "Refactor layout",
					Tags:    nil,
				},
				{
					Commit:  "17f9919",
					Author:  "",
					Date:    "",
					Message: "Remove some excess calls",
					Tags:    nil,
				},
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []LogItem{},
		},
		{
			name: "oneline with no refs",
			input: `386f41d Create LICENSE
3efe836 Update README and add examples`,
			expected: []LogItem{
				{
					Commit:  "386f41d",
					Author:  "",
					Date:    "",
					Message: "Create LICENSE",
					Tags:    nil,
				},
				{
					Commit:  "3efe836",
					Author:  "",
					Date:    "",
					Message: "Update README and add examples",
					Tags:    nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOnelineFormat(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d", len(tt.expected), len(result))
				return
			}

			for i, item := range result {
				expected := tt.expected[i]

				if item.Commit != expected.Commit {
					t.Errorf("Item %d: Expected commit %q, got %q", i, expected.Commit, item.Commit)
				}

				if item.Message != expected.Message {
					t.Errorf("Item %d: Expected message %q, got %q", i, expected.Message, item.Message)
				}

				if !reflect.DeepEqual(item.Tags, expected.Tags) {
					t.Errorf("Item %d: Expected tags %v, got %v", i, expected.Tags, item.Tags)
				}
			}
		})
	}
}

func TestParseMultilineFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []LogItem
	}{
		{
			name: "multiline with tag",
			input: `commit 1d9a11252e834b11eb57582a40446fb6845a8ecb (HEAD -> main, tag: v0.0.6, origin/main, origin/HEAD)
Author: Test User <test@example.com>
Date:   Tue Jun 24 22:40:37 2025 -0400

    Add ls-files command to repo

commit 14d383bff10a065a962f9ec13856d001e091fc88 (tag: v0.0.5)
Author: Test User <test@example.com>
Date:   Tue Jun 24 12:19:43 2025 -0400

    Add diff command to repo`,
			expected: []LogItem{
				{
					Commit:  "1d9a11252e834b11eb57582a40446fb6845a8ecb",
					Author:  "Test User <test@example.com>",
					Date:    "Tue Jun 24 22:40:37 2025 -0400",
					Message: "Add ls-files command to repo",
					Tags:    []string{"v0.0.6"},
				},
				{
					Commit:  "14d383bff10a065a962f9ec13856d001e091fc88",
					Author:  "Test User <test@example.com>",
					Date:    "Tue Jun 24 12:19:43 2025 -0400",
					Message: "Add diff command to repo",
					Tags:    []string{"v0.0.5"},
				},
			},
		},
		{
			name: "multiline with multi-paragraph message",
			input: `commit 5b37f2ef200032c00793f2329989f11f5fc898bf (tag: v0.0.3)
Author: Test User <test@example.com>
Date:   Wed Jun 11 16:20:31 2025 -0400

    Split the git client itself from the repo
    Allows for using a different binary other than one on PATH.
    Add functions for working with remotes, getting hashes and viewing files at specific commits.`,
			expected: []LogItem{
				{
					Commit:  "5b37f2ef200032c00793f2329989f11f5fc898bf",
					Author:  "Test User <test@example.com>",
					Date:    "Wed Jun 11 16:20:31 2025 -0400",
					Message: "Split the git client itself from the repo\nAllows for using a different binary other than one on PATH.\nAdd functions for working with remotes, getting hashes and viewing files at specific commits.",
					Tags:    []string{"v0.0.3"},
				},
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []LogItem{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMultilineFormat(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d", len(tt.expected), len(result))
				return
			}

			for i, item := range result {
				expected := tt.expected[i]

				if item.Commit != expected.Commit {
					t.Errorf("Item %d: Expected commit %q, got %q", i, expected.Commit, item.Commit)
				}

				if item.Author != expected.Author {
					t.Errorf("Item %d: Expected author %q, got %q", i, expected.Author, item.Author)
				}

				if item.Date != expected.Date {
					t.Errorf("Item %d: Expected date %q, got %q", i, expected.Date, item.Date)
				}

				if item.Message != expected.Message {
					t.Errorf("Item %d: Expected message %q, got %q", i, expected.Message, item.Message)
				}

				if !reflect.DeepEqual(item.Tags, expected.Tags) {
					t.Errorf("Item %d: Expected tags %v, got %v", i, expected.Tags, item.Tags)
				}
			}
		})
	}
}
