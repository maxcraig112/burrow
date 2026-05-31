.PHONY: all exchange relay burrow run-exchange run-relay clean

BIN := bin

all: exchange relay burrow

$(BIN):
	mkdir -p $@

exchange: | $(BIN)
	go build -o $(BIN)/ ./cmd/exchange

relay: | $(BIN)
	go build -o $(BIN)/ ./cmd/relay

burrow: | $(BIN)
	go build -o $(BIN)/ ./cmd/burrow

run-exchange: exchange
	./$(BIN)/exchange

run-relay: relay
	./$(BIN)/relay

clean:
	rm -rf $(BIN)
