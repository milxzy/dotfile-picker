// dotpicker-demo shows what the app does without requiring a tty
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/milxzy/dot-generator/internal/config"
	"github.com/milxzy/dot-generator/internal/manifest"
)

func main() {
	fmt.Println("=== dotfile picker demo ===")
	fmt.Println()

	// load config
	cfg, err := config.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// ensure directories exist
	if err := cfg.EnsureDirectories(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("config directories created:\n")
	fmt.Printf("  cache: %s\n", cfg.CacheDir)
	fmt.Printf("  backups: %s\n", cfg.BackupDir)
	fmt.Printf("  logs: %s\n\n", cfg.LogDir)

	// fetch manifest
	fetcher := manifest.NewFetcher(cfg.ManifestURL, cfg.ManifestCachePath)
	ctx := context.Background()

	fmt.Println("fetching manifest from github (or cache)...")
	m, err := fetcher.Fetch(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ loaded %d creators in %d categories\n\n", len(m.Creators), len(m.Categories))

	// show categories
	fmt.Println("=== available categories ===")
	fmt.Println()
	for i, cat := range m.Categories {
		creators := m.GetCreatorsByCategory(cat.ID)
		fmt.Printf("%d. %s (%d creators)\n", i+1, cat.Name, len(creators))
		fmt.Printf("   %s\n\n", cat.Description)
	}

	// show some creators
	fmt.Println("=== featured creators ===")
	fmt.Println()
	for i, creator := range m.Creators {
		if i >= 3 {
			break // just show first 3
		}
		fmt.Printf("● %s\n", creator.Name)
		fmt.Printf("  github: @%s\n", creator.GitHub)
		fmt.Printf("  %s\n", creator.Description)
		fmt.Printf("  dotfiles:\n")
		for _, df := range creator.Dotfiles {
			fmt.Printf("    - %s: %s\n", df.Name, df.Description)
		}
		fmt.Println()
	}

	fmt.Println("=== how to use ===")
	fmt.Println()
	fmt.Println("to use the full interactive tui, run:")
	fmt.Println("  ./bin/dotpicker")
	fmt.Println()
	fmt.Println("navigation:")
	fmt.Println("  - arrow keys to move")
	fmt.Println("  - enter to select")
	fmt.Println("  - esc to go back")
	fmt.Println("  - q to quit")
	fmt.Println()
	fmt.Println("the tui lets you:")
	fmt.Println("  1. browse categories")
	fmt.Println("  2. view creators in each category")
	fmt.Println("  3. select which dotfiles to apply")
	fmt.Println("  4. see diffs before applying")
	fmt.Println("  5. safely apply with automatic backups")
	fmt.Println()
	fmt.Println("note: you need to run this in a real terminal for the tui to work!")
}
