.PHONY: all exchange relay burrow clean

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

clean:
	rm -rf $(BIN)
