GOROOT := $(shell go env GOROOT)
PORT ?= 8080

.PHONY: all client server run test clean

all: client server

# The game itself: Go compiled to WebAssembly, plus Go's JS loader shim.
client:
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/game.wasm ./cmd/client
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" web/ 2>/dev/null || cp "$(GOROOT)/misc/wasm/wasm_exec.js" web/

server:
	go build -o bin/server ./cmd/server

run: all
	./bin/server -port $(PORT)

test:
	go vet ./...
	GOOS=js GOARCH=wasm go vet ./cmd/client
	go test ./...

clean:
	rm -rf bin web/game.wasm web/wasm_exec.js
