# Contributing to ADB Extensions Kit

Thank you for your interest in contributing to ADB Extensions Kit (ak)! This document provides guidelines and information for contributors.

## Welcome

We welcome contributions of all kinds:
- Bug fixes
- New features
- Documentation improvements
- Code quality improvements
- Test coverage

## Getting Started

### Prerequisites

- **Go** 1.27 or newer
- **adb** (Android Debug Bridge) - for running commands against devices
- **golangci-lint** (optional) - the same linter CI runs

### Development Setup

```bash
git clone https://github.com/YOUR_USERNAME/homebrew-adb-extensions.git
cd homebrew-adb-extensions
go run ./cmd/ak --help
```

## Project Structure

```
cmd/ak/              Entry point (sets version, enables ANSI on Windows)
internal/cli/        Commands: one file per command or group, router in cli.go
internal/adb/        Runs adb, finds it (AK_ADB, ANDROID_HOME, PATH, ...), parses common output
internal/adb/adbtest Fake adb runner for tests
internal/tui/        Interactive picker (single/multi select, filter, sort)
internal/apk/        Package name and signer certificates from an APK (no SDK tools)
internal/style/      ANSI color constants
internal/debuglog/   AK_DEBUG=1 logging
completions/_ak      Zsh completion (hand-maintained, checked by a test)
docs/spec/           Command behavior spec
.goreleaser.yaml     Release builds
.github/formula/     Homebrew formula template used at release time
```

## Development Workflow

```bash
git checkout -b feature/your-feature
go run ./cmd/ak <command>      # try it
go test ./...                  # unit tests
go vet ./... && golangci-lint run
goreleaser build --snapshot --clean   # optional: cross-platform build
```

## Adding a New Command

1. Create `internal/cli/<name>.go` with a `*Command` (name, completion description,
   global-help usage/description, accepted flags, run function) and a help function.
   Use `parseLeading` for the usual `-h|--help` and option handling.
2. Add it to `Commands` in `internal/cli/cli.go` (order = global help order).
3. Add the command and its options to `completions/_ak`.
   `TestCompletionMatchesCommands` fails if the completion and the command table differ.
4. Add tests with the fake adb runner (`internal/adb/adbtest`), see `cli_test.go`.
5. Document it in `README.md`, `README.ko.md` and `CHANGELOG.md`.

## Code Style

- `gofmt`, `go vet` and `golangci-lint` must pass (CI runs them).
- Output goes through `say`/`sayf` so tests can capture it; exit with `exit(code)`
  so deferred cleanups (terminal restore, temp files) still run.
- Colors come from `internal/style` (dot-imported: `Red`, `Green`, `NC`, `Error`, ...).
- Keep output identical across platforms; Windows consoles get ANSI enabled at startup.

## Testing

### Debug Mode

```bash
AK_DEBUG=1 ak install
tail -f "${TMPDIR:-/tmp}/ak-debug.log"   # or set AK_DEBUG_LOG=/path/to/log
```

Every adb invocation and picker result is logged with file and line.

### Manual Testing

Unit tests cover parsing and command flows with a fake adb. Also try your change on a real device:

1. **Multiple devices**: 0, 1 and several connected devices
2. **Interactive UI**: arrow keys, Space, `/` filter, `S` sort, Ctrl+C
3. **Edge cases**: empty input, invalid input, missing packages

### Testing Checklist

- [ ] `go test ./...` passes
- [ ] Command works with no devices, one device and multiple devices
- [ ] Help message displays correctly (`ak command --help`)
- [ ] Tab completion works (`completions/_ak` updated)
- [ ] Error messages are clear and helpful

## Pull Request Process

1. **Update Documentation**: Update README.md and CHANGELOG.md if needed
2. **Test Thoroughly**: Run through the testing checklist
3. **Commit Messages**: Use conventional commit format:
   - `feat:` - New feature
   - `fix:` - Bug fix
   - `docs:` - Documentation changes
   - `refactor:` - Code refactoring
   - `test:` - Test changes
   - `chore:` - Maintenance tasks

4. **Create Pull Request**: 
   - Provide a clear description of changes
   - Reference any related issues
   - Include screenshots if UI changes are involved

5. **Code Review**: Address any feedback from maintainers

## Community Guidelines

- Be respectful and constructive
- Help others when you can
- Report bugs with clear reproduction steps
- Suggest features with use cases
- Write clear commit messages

## Questions?

- **Issues**: [GitHub Issues](https://github.com/luminousvault/homebrew-adb-extensions/issues)
- **Discussions**: [GitHub Discussions](https://github.com/luminousvault/homebrew-adb-extensions/discussions)

Thank you for contributing to ADB Extensions Kit!
