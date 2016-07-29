# Set an output prefix, which is the local directory if not specified
PREFIX?=$(shell pwd)

# Get the git commit
GIT_COMMIT="$(shell git rev-parse HEAD)"
GIT_DIRTY="$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)"

GO_LDFLAGS=-ldflags "-X github.com/fairfaxmedia/flywheel.GitCommit=${GIT_COMMIT}${GIT_DIRTY}"

.PHONY: clean all fmt vet lint build test bin
.DEFAULT: default
all: AUTHORS clean fmt vet fmt lint build test bin

AUTHORS: .git/HEAD
	 git log --format='%aN <%aE>' | sort -fu > $@

${PREFIX}/bin/flywheel: $(shell find . -type f -name '*.go')
	@echo "+ $@"
	@go build -o $@ ${GO_LDFLAGS} ./cmd/flywheel

vet: bin
	@echo "+ $@"
	@go vet ./...

fmt:
	@echo "+ $@"
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || \
		echo "+ please format Go code with 'gofmt -s'"

lint:
	@echo "+ $@"
	@test -z "$$(go list ./... | grep -v /vendor/ | xargs -L 1 golint | tee /dev/stderr)"

build:
	@echo "+ $@"
	@go build -x ${GO_LDFLAGS} ./...

test:
	@echo "+ $@"
	@test -z "$$(go list ./... | grep -v /vendor/ | xargs -L 1 go test | tee /dev/stderr)"

bin: ${PREFIX}/bin/flywheel
	@echo "+ $@"

clean:
	@echo "+ $@"
	@rm -rf "${PREFIX}/bin/flywheel"
