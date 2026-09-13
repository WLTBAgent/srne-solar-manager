# SRNE Solar Manager — GUI, CLI, and simulator for SRNE charge controllers.
#
# Common tasks:
#   make              build all binaries into ./bin
#   make test         run the test suite
#   sudo make install install the binaries to /usr/local/bin
#   make demo         start the simulator and open the GUI against it
#
# See `make help` for every target.

GO        ?= go
BIN_DIR   ?= bin
PREFIX    ?= /usr/local
BINDIR    ?= $(PREFIX)/bin
DESTDIR   ?=
ARGS      ?=

BINARIES  := charge-controller charge-controller-gui devsim
SRC       := $(shell find cmd internal -name '*.go' -not -name '*_test.go')

.DEFAULT_GOAL := help
.PHONY: all build test test-race cover vet fmt fmt-check check install uninstall clean run sim demo tidy help

all: build

## build: compile all binaries into ./bin
build: $(BINARIES:%=$(BIN_DIR)/%)

$(BIN_DIR)/%: $(SRC)
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $@ ./cmd/$*

## test: run the test suite
test:
	$(GO) test ./...

## test-race: run the test suite with the race detector
test-race:
	$(GO) test -race ./...

## cover: run tests with coverage summary
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

## vet: run go vet
vet:
	$(GO) vet ./...

## fmt: rewrite all sources with gofmt
fmt:
	gofmt -w cmd internal

## fmt-check: fail if any source is not gofmt-clean
fmt-check:
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

## check: everything CI should run (fmt-check, vet, tests)
check: fmt-check vet test

## install: build and install binaries to the install dir (default /usr/local/bin; sudo usually needed)
install: build
	install -d $(DESTDIR)$(BINDIR)
	@for b in $(BINARIES); do \
		install -m 0755 $(BIN_DIR)/$$b $(DESTDIR)$(BINDIR)/$$b; \
		echo "installed $$b -> $(DESTDIR)$(BINDIR)/$$b"; \
	done

## uninstall: remove the installed binaries
uninstall:
	@for b in $(BINARIES); do \
		rm -f $(DESTDIR)$(BINDIR)/$$b; \
		echo "removed $(DESTDIR)$(BINDIR)/$$b"; \
	done

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out

## run: build and start the GUI (pass flags with ARGS="-port ... -connect")
run: $(BIN_DIR)/charge-controller-gui
	$(BIN_DIR)/charge-controller-gui $(ARGS)

## sim: start the virtual controller (prints a /dev/pts/N port to use)
sim:
	$(GO) run ./cmd/devsim

## demo: start the simulator and open the GUI connected to it
demo: build
	@log=$$(mktemp); \
	$(BIN_DIR)/devsim > $$log 2>&1 & \
	sim=$$!; \
	sleep 1; \
	port=$$(grep -o '/dev/pts/[0-9]*' $$log | head -1); \
	if [ -z "$$port" ]; then \
		echo "devsim failed to start:"; cat $$log; kill $$sim 2>/dev/null; exit 1; \
	fi; \
	echo "Simulator on $$port — closing the GUI stops the demo."; \
	$(BIN_DIR)/charge-controller-gui -port $$port -connect $(ARGS); \
	kill $$sim 2>/dev/null || true

## tidy: sync go.mod / go.sum
tidy:
	$(GO) mod tidy

## help: show this help
help:
	@echo "SRNE Solar Manager"
	@echo
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' ' \
		NR == 1 { print "Targets:" } \
		{ printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }'
	@echo
	@echo "Variables:"
	@echo "  ARGS      extra flags for run/demo (e.g. ARGS=\"-tab 3\")"
	@echo "  PREFIX    install prefix (default /usr/local)"
	@echo "  BINDIR    install dir relative to PREFIX (default \$$PREFIX/bin)"
	@echo "  DESTDIR   staging root for packaging (e.g. make install DESTDIR=/tmp/pkg)"
