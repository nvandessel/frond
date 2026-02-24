.PHONY: build test lint clean

build:
	go build -o frond .

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f frond
