BINARY := pr-agents
PKG := ./...

.PHONY: build test vet fmt fmt-check ci

build:
	go build -o bin/$(BINARY) .

test:
	go test $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

# Fail (printing the offending files) if anything is not gofmt-clean.
fmt-check:
	@! gofmt -l . | grep .

# The full gate: formatting, vet, and tests.
ci: fmt-check vet test
