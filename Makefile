.PHONY: all test lint wasm cli site fuzz clean

all: wasm cli site

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && false)
	go vet ./engine ./cmd/shadegrove
	GOOS=js GOARCH=wasm go vet ./cmd/wasm

test:
	go test -race -count=1 ./...

fuzz:
	go test -run=NONE -fuzz=FuzzParseScene -fuzztime=30s ./engine

wasm:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o site/assets/shadegrove.wasm ./cmd/wasm

cli:
	go build -trimpath -o build/shadegrove ./cmd/shadegrove

site:
	npm run build

clean:
	rm -rf build site/assets/shadegrove.wasm
