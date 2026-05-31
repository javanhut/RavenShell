BINARY      := ravenshell
PREFIX      ?= /usr/local
BINDIR      := $(PREFIX)/bin
INSTALL_BIN := $(DESTDIR)$(BINDIR)/$(BINARY)
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -X main.version=$(VERSION)

.PHONY: all build install uninstall test vet fmt clean register-shell set-default help

all: build

## build: compile the ravenshell binary
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## install: build and install to $(PREFIX)/bin (override with PREFIX=...)
install: build
	@mkdir -p "$(DESTDIR)$(BINDIR)"
	install -m 0755 $(BINARY) "$(INSTALL_BIN)"
	@echo "Installed $(BINARY) to $(INSTALL_BIN)"

## uninstall: remove the installed binary
uninstall:
	rm -f "$(INSTALL_BIN)"
	@echo "Removed $(INSTALL_BIN)"

## test: run the test suite
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go source
fmt:
	gofmt -w .

## clean: remove the built binary
clean:
	rm -f $(BINARY)

## register-shell: add the installed binary to /etc/shells (needs sudo)
register-shell:
	@grep -qxF "$(BINDIR)/$(BINARY)" /etc/shells 2>/dev/null \
		|| echo "$(BINDIR)/$(BINARY)" | sudo tee -a /etc/shells >/dev/null
	@echo "$(BINDIR)/$(BINARY) is registered in /etc/shells"

## set-default: make RavenShell your login shell (run 'make register-shell' first)
set-default:
	chsh -s "$(BINDIR)/$(BINARY)"
	@echo "Default shell set to $(BINDIR)/$(BINARY). Log out and back in to apply."

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
