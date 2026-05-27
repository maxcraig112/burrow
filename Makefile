.PHONY: all exchange relay wormhole clean

BIN := bin

all: exchange relay wormhole

$(BIN):
	mkdir -p $@

exchange: | $(BIN)
	go build -o $(BIN)/ ./cmd/exchange

relay: | $(BIN)
	go build -o $(BIN)/ ./cmd/relay

wormhole: | $(BIN)
	go build -o $(BIN)/ ./cmd/wormhole

clean:
	rm -rf $(BIN)
