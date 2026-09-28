BIN := moneybot
PKG := ./cmd/bot
DB  := ./moneybot.db

.PHONY: build run test vet fmt clean db

build:
	go build -o $(BIN) $(PKG)

run: build
	./$(BIN)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -f $(BIN)

db:
	sqlite3 $(DB) ".tables"
