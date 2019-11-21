PROJECT_NAME := restic
PROJECT_MAIN := ./cmd/restic
BINARY_NAME := restic
DOCKER_REPO := restic/restic
DOCKER_FILE := docker/Dockerfile

.PHONY: all clean get test build run snapshot

all: get test build

# Common actions
clean:
	rm -f $(BINARY_NAME) # Output binary file
	rm -rf dist/ # Goreleaser dist folder

get:
	go get -t

test: get
	go test ./...

build: get
	go build -o $(BINARY_NAME) $(PROJECT_MAIN)

run: build
	./$(BINARY_NAME)

# Release actions
snapshot: get
	goreleaser --snapshot

# Docker actions
docker-build:
	docker build --rm -t $(DOCKER_REPO):latest -f $(DOCKER_FILE) .
