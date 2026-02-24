BINARY = tramuntana

.PHONY: build install clean vet test

build:
	go build -o $(BINARY) ./cmd/tramuntana
	go install ./cmd/tramuntana

install:
	go install ./cmd/tramuntana

clean:
	rm -f $(BINARY)

vet:
	go vet ./...

test:
	go test ./...
