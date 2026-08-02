# Targets for the checks CI runs, so that a contributor runs the same thing
# locally. There is nothing to build: this is a library.

GO ?= go
FUZZTIME ?= 60s

.PHONY: all
all: fmt lint test

.PHONY: test
test:
	$(GO) test ./...

.PHONY: race
race:
	$(GO) test -race ./...

.PHONY: cover
cover:
	$(GO) test -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO) tool cover -func=coverage.txt | tail -1

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	gofmt -l -w .

# The official HL7 suite on its own, which is the conformance question.
.PHONY: conformance
conformance:
	$(GO) test -v -run TestFunctional ./internal/engine/

# Longer than CI runs them. Both targets have found real bugs.
.PHONY: fuzz
fuzz:
	$(GO) test -run XXX -fuzz FuzzValidate -fuzztime $(FUZZTIME) ./internal/engine/
	$(GO) test -run XXX -fuzz FuzzComposeRoundTrip -fuzztime $(FUZZTIME) ./internal/engine/

.PHONY: bench
bench:
	$(GO) test -bench . -benchtime 2000x -run XXX ./internal/engine/

.PHONY: vuln
vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

# What a consumer sees. Review it before releasing: an accidental export is
# forever under semver.
.PHONY: api
api:
	@$(GO) doc -all . | grep -E '^(func|type|var|const) ' | sort
	@echo '--- fhir ---'
	@$(GO) doc -all ./fhir | grep -E '^(func|type|var|const) ' | sort
