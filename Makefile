.PHONY: build run clean test install

# Binary name
BINARY=dj

# Build
build:
	go build -o $(BINARY) ./cmd

# Run directly
run:
	go run ./cmd $(ARGS)

# Install to GOPATH/bin
install:
	go install ./cmd

# Clean
clean:
	rm -f $(BINARY)

# Test
test:
	go test -v ./...

# Dependencies
deps:
	go mod download
	go mod tidy

# Format
fmt:
	go fmt ./...

# Examples
example:
	@echo "Examples:"
	@echo "  make run ARGS='\"Daft Punk Around The World\"'"
	@echo "  make run ARGS='-f songs.txt -o ~/Music'"
	@echo "  ./dj \"Song Name\""
	@echo "  ./dj -f playlist.txt"
