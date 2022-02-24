version := v1.0.0

format:
		goimports -w -l .
		go fmt
		gofumpt -w .

license-check:
	# go install github.com/vogo/license-header-checker/cmd/license-header-checker@latest
	license-header-checker -v -a -r apache-license.txt . go

check: license-check
		golangci-lint run

test:
		go test -coverprofile=coverage.txt -covermode=atomic

build: format check test

