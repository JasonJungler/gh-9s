// Command gh-9s is a k9s-style terminal UI for GitHub Actions, shipped as a gh extension.
package main

import (
	"fmt"
	"os"

	"github.com/MurabitoB/gh-9s/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-9s:", err)
		os.Exit(1)
	}
}
