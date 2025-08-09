GO ?= go
SHELL = bash
PKG = github.com/packetd/packetd

VERSION := $(shell cat VERSION)

.PHONY: help
help:
	@echo "Make Targets: "
	@echo " mod: Download and tidy dependencies"
	@echo " lint: Lint Go code"
	@echo " test: Run unit tests"
	@echo " build: Build Go package"
	@echo " install-tools: Install dev tools"

.PHONY: lint
lint:
	gofumpt -w .
	goimports-reviser -project-name "github.com/packetd/packetd" ./...
	find ./ -type f \( -iname \*.go -o -iname \*.sh \) | xargs addlicense -v -f LICENSE

.PHONY: test
test:
	$(GO) test ./... -buildmode=pie -parallel=4 -cover

.PHONY: mod
mod:
	$(GO) mod download
	$(GO) mod tidy

.PHONY: install-tools
tools:
	$(GO) install mvdan.cc/gofumpt@latest
	$(GO) install github.com/incu6us/goimports-reviser/v3@v3.1.1
	$(GO) install github.com/google/addlicense@latest

.PHONY: build
build:
	$(GO) build -ldflags " \
	-s -w \
	-X $(PKG)/common.buildVersion=$(VERSION) \
	-X $(PKG)/common.buildTime=$(shell date -u '+%Y-%m-%d_%I:%M:%S%p') \
	-X $(PKG)/common.buildGitHash=$(shell git rev-parse HEAD)" \
	-o packetd .

.PHONY: push-images
images:
	docker build -t chenjiandongx/packetd:$(VERSION) .
	docker push chenjiandongx/packetd:$(VERSION)
