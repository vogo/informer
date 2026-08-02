version := v1.0.0

# formatting is driven by the same golangci-lint configuration CI lints with,
# so local formatting and CI expectations never diverge.
format:
		golangci-lint fmt ./...
		cd cmd/informer-ui && golangci-lint fmt ./...

license-check:
	# go install github.com/vogo/license-header-checker/cmd/license-header-checker@latest
	license-header-checker -v -a -r apache-license.txt . go

check: license-check
		golangci-lint run

test:
		go test ./... -coverprofile=coverage.txt -covermode=atomic

build: format test
