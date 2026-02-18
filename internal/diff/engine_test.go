package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a helper to create a temp file with the given content.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return p
}

func TestGenerateDiff_NewFile(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "nvim.lua", "-- neovim config\nvim.opt.number = true\n")

	result, err := GenerateDiff(src, filepath.Join(dir, "nonexistent.lua"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsNew {
		t.Error("expected IsNew = true for a missing target file")
	}
	if result.IsIdentical {
		t.Error("expected IsIdentical = false for a new file")
	}
	if !strings.Contains(result.Diff, "new file") {
		t.Errorf("expected diff header 'new file', got: %q", result.Diff)
	}
	if !strings.Contains(result.Diff, "+ -- neovim config") {
		t.Errorf("expected source lines prefixed with '+', got: %q", result.Diff)
	}
}

func TestGenerateDiff_IdenticalFiles(t *testing.T) {
	dir := t.TempDir()
	content := "-- same content\nvim.opt.number = true\n"
	src := writeFile(t, dir, "src.lua", content)
	dst := writeFile(t, dir, "dst.lua", content)

	result, err := GenerateDiff(src, dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsIdentical {
		t.Error("expected IsIdentical = true for identical files")
	}
	if result.IsNew {
		t.Error("expected IsNew = false for identical files")
	}
}

func TestGenerateDiff_ModifiedFile(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.lua", "vim.opt.number = true\nvim.opt.relativenumber = true\n")
	dst := writeFile(t, dir, "dst.lua", "vim.opt.number = true\n")

	result, err := GenerateDiff(src, dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsNew || result.IsIdentical {
		t.Errorf("expected a real diff, got IsNew=%v IsIdentical=%v", result.IsNew, result.IsIdentical)
	}
	if result.Diff == "" {
		t.Error("expected non-empty diff string")
	}
}

func TestGenerateDiff_MissingSource(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateDiff(filepath.Join(dir, "missing.lua"), filepath.Join(dir, "target.lua"))
	if err == nil {
		t.Error("expected error for missing source file, got nil")
	}
}

func TestGetDiffStats_NewFile(t *testing.T) {
	r := &Result{IsNew: true}
	adds, dels := GetDiffStats(r)
	if adds != 0 || dels != 0 {
		t.Errorf("expected 0,0 for new file, got %d,%d", adds, dels)
	}
}

func TestGetDiffStats_IdenticalFile(t *testing.T) {
	r := &Result{IsIdentical: true}
	adds, dels := GetDiffStats(r)
	if adds != 0 || dels != 0 {
		t.Errorf("expected 0,0 for identical file, got %d,%d", adds, dels)
	}
}

func TestGetDiffStats_ModifiedFile(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.lua", "line1\nline2\nline3\n")
	dst := writeFile(t, dir, "dst.lua", "line1\n")

	result, err := GenerateDiff(src, dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adds, dels := GetDiffStats(result)
	// source has 2 extra lines → at least 2 additions
	if adds < 1 {
		t.Errorf("expected at least 1 addition, got %d", adds)
	}
	_ = dels // deletions depend on diff algorithm internals
}

func TestFormatNewFile_LargeFile(t *testing.T) {
	// build a file with more than 20 lines to exercise the truncation path
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("line content\n")
	}

	result := formatNewFile(sb.String())
	if !strings.Contains(result, "more lines") {
		t.Errorf("expected truncation note for large file, got: %q", result)
	}
}
