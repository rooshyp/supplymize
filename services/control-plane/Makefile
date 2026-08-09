.PHONY: build run test test-race cover fmt vet clean docker

BINARY := bin/control-plane
VERSION ?= development

build:
	go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(BINARY) ./cmd/control-plane

run:
	go run -ldflags="-X main.version=$(VERSION)" ./cmd/control-plane

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

vet:
	go vet ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t supplymize/control-plane:$(VERSION) .

clean:
	rm -f $(BINARY) coverage.out
