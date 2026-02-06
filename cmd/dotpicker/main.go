// dotpicker is a tui for browsing and applying dotfiles from content creators
package main

import (
	"fmt"
	"os"

	"github.com/milxzy/dot-generator/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
