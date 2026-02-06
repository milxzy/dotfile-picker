// package diff handles generating and formatting file diffs
package diff

import (
	"fmt"
	"os"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Result represents the result of comparing two files
type Result struct {
	SourcePath  string
	TargetPath  string
	Diff        string
	IsNew       bool // true if target doesn't exist yet
	IsIdentical bool
}

// GenerateDiff creates a diff between source and target files
// source is the new config from the creator
// target is the user's existing config (may not exist)
func GenerateDiff(sourcePath, targetPath string) (*Result, error) {
	result := &Result{
		SourcePath: sourcePath,
		TargetPath: targetPath,
	}

	// read source file
	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("couldn't read source file: %w", err)
	}

	// check if target exists
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// target doesn't exist - this is a new file
			result.IsNew = true
			result.Diff = formatNewFile(string(sourceData))
			return result, nil
		}
		return nil, fmt.Errorf("couldn't read target file: %w", err)
	}

	// both files exist - generate diff
	sourceStr := string(sourceData)
	targetStr := string(targetData)

	if sourceStr == targetStr {
		result.IsIdentical = true
		return result, nil
	}

	// use diffmatchpatch for nice diff output
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(targetStr, sourceStr, false)

	// clean up diff for better readability
	diffs = dmp.DiffCleanupSemantic(diffs)

	result.Diff = formatDiff(diffs)
	return result, nil
}

// formatDiff converts diff operations to a readable string
func formatDiff(diffs []diffmatchpatch.Diff) string {
	var builder strings.Builder

	for _, diff := range diffs {
		text := diff.Text

		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			// additions from the new config
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if line != "" {
					builder.WriteString("+ ")
					builder.WriteString(line)
					builder.WriteString("\n")
				}
			}
		case diffmatchpatch.DiffDelete:
			// deletions from the old config
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if line != "" {
					builder.WriteString("- ")
					builder.WriteString(line)
					builder.WriteString("\n")
				}
			}
		case diffmatchpatch.DiffEqual:
			// unchanged lines - show a few for context
			lines := strings.Split(text, "\n")
			// show first 2 and last 2 lines for context
			if len(lines) > 6 {
				for i := 0; i < 2; i++ {
					if lines[i] != "" {
						builder.WriteString("  ")
						builder.WriteString(lines[i])
						builder.WriteString("\n")
					}
				}
				builder.WriteString("  ...\n")
				for i := len(lines) - 2; i < len(lines); i++ {
					if lines[i] != "" {
						builder.WriteString("  ")
						builder.WriteString(lines[i])
						builder.WriteString("\n")
					}
				}
			} else {
				for _, line := range lines {
					if line != "" {
						builder.WriteString("  ")
						builder.WriteString(line)
						builder.WriteString("\n")
					}
				}
			}
		}
	}

	return builder.String()
}

// formatNewFile formats a new file for display
func formatNewFile(content string) string {
	var builder strings.Builder
	builder.WriteString("new file - doesn't exist yet\n\n")

	lines := strings.Split(content, "\n")
	// show first 20 lines as preview
	maxLines := 20
	if len(lines) > maxLines {
		for i := 0; i < maxLines; i++ {
			builder.WriteString("+ ")
			builder.WriteString(lines[i])
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("\n... and %d more lines", len(lines)-maxLines))
	} else {
		for _, line := range lines {
			builder.WriteString("+ ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// GetDiffStats returns statistics about the diff
func GetDiffStats(result *Result) (additions, deletions int) {
	if result.IsNew || result.IsIdentical {
		return 0, 0
	}

	lines := strings.Split(result.Diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+ ") {
			additions++
		} else if strings.HasPrefix(line, "- ") {
			deletions++
		}
	}

	return additions, deletions
}
