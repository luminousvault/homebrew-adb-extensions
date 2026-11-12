# Local build and install. Releases use GoReleaser (.goreleaser.yaml).
#
#   make              build build/ak
#   make install      build, then copy ak and the zsh completion under PREFIX
#   make uninstall    remove them
#
# PREFIX defaults to /usr/local; sudo is used only when its directories are not writable.

PREFIX  ?= /usr/local
BINDIR  := $(PREFIX)/bin
ZSHDIR  := $(PREFIX)/share/zsh/site-functions

VERSION := $(shell cat VERSION)-dev
DATE    := $(shell date +%Y-%m-%d)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.date=$(DATE)
# sudo if a target directory (or its nearest existing parent) is not writable.
SUDO    := $(shell for d in $(BINDIR) $(ZSHDIR); do \
             while [ ! -e "$$d" ]; do d=$$(dirname "$$d"); done; \
             [ -w "$$d" ] || { echo sudo; break; }; done)

.PHONY: build install uninstall completions FORCE

build: build/ak

# Always run go build: its cache makes this cheap, and a build/ak made by a plain
# `go build` would otherwise be installed without the version flags.
build/ak: FORCE
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/ak

install: build/ak
	$(SUDO) install -d $(BINDIR) $(ZSHDIR)
	$(SUDO) install -m 755 build/ak $(BINDIR)/ak
	$(SUDO) install -m 644 completions/_ak $(ZSHDIR)/_ak
	@echo "Installed $(BINDIR)/ak and $(ZSHDIR)/_ak"
	@echo "Run 'exec zsh' in open terminals to load the new completion"
	@echo "(first install with oh-my-zsh: rm -f ~/.zcompdump* && exec zsh)"

# completions/_ak is generated from the command table (a test checks it).
completions: build/ak
	build/ak completion zsh > completions/_ak

uninstall:
	$(SUDO) rm -f $(BINDIR)/ak $(ZSHDIR)/_ak
	@echo "Removed $(BINDIR)/ak and $(ZSHDIR)/_ak"

FORCE:
