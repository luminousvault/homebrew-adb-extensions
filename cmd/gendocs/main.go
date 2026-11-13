// Command gendocs prints the command reference (docs/commands.md).
//
//	go run ./cmd/gendocs > docs/commands.md
package main

import (
	"fmt"

	"github.com/luminousvault/homebrew-adb-extensions/internal/cli"
)

func main() {
	fmt.Print(cli.CommandsMarkdown())
}
