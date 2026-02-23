.PHONY: build test lint clean

build:
	go build -o tier .

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f tier
