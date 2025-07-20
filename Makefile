.PHONY: build run-review run-review-verbose test clean develop develop-reflex

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=livereview

build:
	$(GOBUILD) -o $(BINARY_NAME)

develop:
	which air || go install github.com/air-verse/air@latest
	air

develop-reflex:
	which reflex || go install github.com/cespare/reflex@latest
	reflex -r '\.go$$' -s -- sh -c 'go build -o $(BINARY_NAME) && ./$(BINARY_NAME) api'

run-review:
	./$(BINARY_NAME) review --dry-run https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

run-review-verbose:
	./$(BINARY_NAME) review --dry-run --verbose https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
