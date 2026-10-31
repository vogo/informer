.PHONY: format license-check lint lint-update check test build cli

LINT_BUILD_TAGS := $(if $(filter Linux,$(shell uname -s)),gtk3,)

version := v1.0.0

# formatting is driven by the same golangci-lint configuration CI lints with,
# so local formatting and CI expectations never diverge.
format:
		golangci-lint fmt ./...
		cd cmd/informer-ui && golangci-lint fmt ./...

license-check:
	# go install github.com/vogo/license-header-checker/cmd/license-header-checker@latest
	license-header-checker -v -a -r apache-license.txt . go

lint-update:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

lint:
	golangci-lint run --timeout=10m
	cd cmd/informer-ui && golangci-lint run --timeout=10m --build-tags=$(LINT_BUILD_TAGS)

check: license-check lint

test:
		go test ./... -coverprofile=coverage.txt -covermode=atomic

build: format test

cli:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o informer ./cmd/informer
