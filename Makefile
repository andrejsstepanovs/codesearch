.PHONY: build
build:
	CGO_CFLAGS="-Wno-deprecated-declarations" go build -o ./bin/codesearch .

.PHONY: install
install:
	go mod tidy
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2

.PHONY: run-lint
run-lint:
	golangci-lint run ./...

.PHONY: run-test
run-test:
	CGO_CFLAGS="-Wno-deprecated-declarations" go test -short ./...
