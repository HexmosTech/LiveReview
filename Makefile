.PHONY: build run-review run-review-verbose test clean

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=livereview

build:
	$(GOBUILD) -o $(BINARY_NAME)

run-review:
	./$(BINARY_NAME) review --dry-run https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

run-review-verbose:
	./$(BINARY_NAME) review --dry-run --verbose https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
