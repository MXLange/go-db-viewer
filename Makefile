SHELL := /bin/sh
.DEFAULT_GOAL := help
.NOTPARALLEL:

VERSION ?= 0.1.0
ARCH ?= $(shell go env GOHOSTARCH)
WAILS_VERSION := v2.15.0
NFPM_VERSION := v2.48.0
TOOLS_DIR := $(CURDIR)/build/tools
WAILS_BIN := $(TOOLS_DIR)/wails-$(WAILS_VERSION)/wails
NFPM_BIN := $(TOOLS_DIR)/nfpm-$(NFPM_VERSION)/nfpm

export APP_VERSION := $(VERSION)
export APP_ARCH := $(ARCH)
export WAILS_BIN NFPM_BIN

.PHONY: help tools build-linux build-exe build-deb build-rpm build-mac check-linux check-mac

help:
	@printf '%s\n' \
	  'make build-exe     Gera .exe para Windows, a partir de Linux ou macOS.' \
	  'make build-deb     Gera pacote Debian/Ubuntu no Linux.' \
	  'make build-rpm     Gera pacote RPM no Linux.' \
	  'make build-mac     Gera .app e .dmg no macOS, com suporte JDBC ao Ignite.' \
	  'make build-linux   Gera somente o executável Linux.' \
	  'make tools         Instala Wails e nFPM em build/tools, sem alterar o PATH.' \
	  'Variáveis: VERSION=0.1.0 ARCH=amd64 (ou arm64).' \
	  'Artefatos: build/bin/.'

tools: $(WAILS_BIN) $(NFPM_BIN)

$(WAILS_BIN):
	@mkdir -p "$(dir $(WAILS_BIN))"
	GOOS="$(shell go env GOHOSTOS)" GOARCH="$(shell go env GOHOSTARCH)" GOBIN="$(dir $(WAILS_BIN))" go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

$(NFPM_BIN):
	@mkdir -p "$(dir $(NFPM_BIN))"
	GOOS="$(shell go env GOHOSTOS)" GOARCH="$(shell go env GOHOSTARCH)" GOBIN="$(dir $(NFPM_BIN))" go install github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VERSION)

check-linux:
	@sh scripts/build.sh check-linux

check-mac:
	@sh scripts/build.sh check-mac

build-linux: check-linux $(WAILS_BIN)
	@sh scripts/build.sh linux

build-exe: $(WAILS_BIN)
	@sh scripts/build.sh exe

build-deb: check-linux $(NFPM_BIN) build-linux
	@sh scripts/build.sh deb

build-rpm: check-linux $(NFPM_BIN) build-linux
	@sh scripts/build.sh rpm

build-mac: check-mac $(WAILS_BIN)
	@sh scripts/build.sh mac
