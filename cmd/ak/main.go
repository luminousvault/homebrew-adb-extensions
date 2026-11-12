// Command ak is the ADB Extensions Kit CLI.
package main

import (
	"os"

	"github.com/luminousvault/homebrew-adb-extensions/internal/cli"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
)

// Set by goreleaser ldflags.
var (
	version = "dev"
	date    = "unknown"
)

func main() {
	cli.Version, cli.ReleaseDate = version, date
	enableVT()
	debuglog.Init()
	os.Exit(cli.Main(os.Args[1:]))
}
