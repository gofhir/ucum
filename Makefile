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

# Lo que hay que correr antes de dar algo por terminado. A diferencia de
# `all`, no reescribe nada: falla si hay archivos sin formatear.
.PHONY: check
check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...
	golangci-lint run ./...
	$(GO) test ./...

# Las cantidades UCUM de los ejemplos oficiales de R4, R4B y R5. Descarga los
# packages (unos 54 MB, con SHA-256 fijado en scripts/packages.sha256) a
# .cache/packages/; por eso va aparte de `check`. También comprueba que la
# parte FHIR de los casos de Java siga al día, que `check` no puede ver.
.PHONY: check-packages
check-packages:
	scripts/fetch-packages.sh
	UCUM_PACKAGES=$(CURDIR)/.cache/packages $(GO) test -count=1 -run TestOfficialExamples -v ./fhir/
	UCUM_PACKAGES=$(CURDIR)/.cache/packages $(GO) test -count=1 -run '^TestJavaCasesFresh$$' -v ./internal/engine/

# Prueba los oráculos: cada defecto inyectado en una copia del repo tiene que
# hacer fallar su test (docs/plans/001-canonicalkey-exact.md, §7). Necesita
# los packages, como check-packages, porque la línea base corre ese test.
.PHONY: mutations
mutations:
	scripts/fetch-packages.sh
	python3 -I scripts/mutations.py $(CURDIR)/.cache/packages

# Regenera la salida versionada de Ucum-java en internal/engine/testdata/java/
# (plan docs/plans/002-compare-java.md). Necesita un JDK 17+ y red; no corre en
# CI, que compara contra lo versionado.
.PHONY: java-dump
java-dump:
	scripts/java-dump.sh

# La comparación contra Ucum-java con su reporte, que queda con fecha en
# docs/plans/evidence/. No necesita Java.
.PHONY: compare-java
compare-java:
	UCUM_JAVA_REPORT=$(CURDIR)/docs/plans/evidence/002-$$(date +%Y-%m-%d)-compare-java.md \
		$(GO) test -count=1 -run '^TestCompareJava$$' -v ./internal/engine/

# The official HL7 suite on its own, which is the conformance question.
.PHONY: conformance
conformance:
	$(GO) test -v -run TestFunctional ./internal/engine/

# Longer than CI runs them. The first two have found real bugs.
.PHONY: fuzz
fuzz:
	$(GO) test -run XXX -fuzz FuzzValidate -fuzztime $(FUZZTIME) ./internal/engine/
	$(GO) test -run XXX -fuzz FuzzComposeRoundTrip -fuzztime $(FUZZTIME) ./internal/engine/
	$(GO) test -run XXX -fuzz FuzzProductUnits -fuzztime $(FUZZTIME) ./internal/engine/

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
