// dotpicker is a tui for browsing and applying dotfiles from content creators
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/milxzy/dotfile-picker/internal/config"
	"github.com/milxzy/dotfile-picker/internal/logger"
	"github.com/milxzy/dotfile-picker/internal/tui"
)

func main() {
	// parse command line flags
	verbose := flag.Bool("verbose", false, "enable verbose debug logging to terminal")
	flag.Parse()

	// load config to get log directory
	cfg, err := config.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// initialize logger
	if err := logger.Init(cfg.LogDir, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error initializing logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// run the TUI
	if err := tui.Run(); err != nil {
		logger.Error("Application error: %v", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
