APP     := ainsel-mcp
IMAGE   := localhost:30500/ainsel/$(APP)
TAG     ?= latest

.PHONY: build test lint image push

build:
	go build -o bin/$(APP) ./cmd/server

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./...

image:
	docker build -t $(IMAGE):$(TAG) .

push: image
	docker push $(IMAGE):$(TAG)
