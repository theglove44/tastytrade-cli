BINARY := tt
MODULE := github.com/theglove44/tastytrade-cli
BUILD_FLAGS := -ldflags="-s -w"

.PHONY: build test lint clean run-sandbox env-check closeout

## Build the binary
build:
	go build $(BUILD_FLAGS) -o $(BINARY) .

## Run tests (unit + integration stubs)
test:
	go test ./... -race -timeout 60s

## Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

## Clean build artifacts
clean:
	rm -f $(BINARY)
	go clean -testcache

## Quick smoke against sandbox (requires .env sourced)
run-sandbox: build
	@echo "Running against sandbox..."
	./$(BINARY) accounts --verbose

## Confirm all required env vars are set
env-check:
	@[ -n "$(TASTYTRADE_CLIENT_ID)" ]   || (echo "TASTYTRADE_CLIENT_ID not set" && exit 1)
	@[ -n "$(TASTYTRADE_ACCOUNT_ID)" ]  || (echo "TASTYTRADE_ACCOUNT_ID not set" && exit 1)
	@[ -n "$(TASTYTRADE_BASE_URL)" ]    || (echo "TASTYTRADE_BASE_URL not set" && exit 1)
	@echo "✓ Required env vars set"

## Close out the current branch with a generated commit message
closeout:
	@[ -n "$(MESSAGE)" ] || (echo "MESSAGE not set" && exit 1)
	@[ -n "$(FILES)" ] || (echo "FILES not set" && exit 1)
	@./scripts/branch_closeout.sh --message "$(MESSAGE)" $(FILES)

## Arm the kill switch
kill:
	./$(BINARY) kill

## Disarm the kill switch
resume:
	./$(BINARY) resume
