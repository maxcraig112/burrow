# Building from source & local dev

## Prerequisites

- Go 1.27+
- [Task](https://taskfile.dev) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- [ko](https://ko.build) — only for building container images (`go install github.com/google/ko@latest`)

## Build

```bash
# All three binaries into bin/
task

# Or individually
task exchange
task relay
task burrow
```

## Test

```bash
task test   # unit tests
task e2e    # end-to-end tests (in-process exchange + relay)
```

## Run servers locally

Both servers load `.env` from the working directory. The repo ships with defaults that work out of the box for local use:

```bash
# Terminal 1 — exchange on :8080
./bin/exchange

# Terminal 2 — relay on :9090 (TCP) and :8082 (HTTP tunnel)
./bin/relay
```

Then run the CLI against them:

```bash
./bin/burrow send photo.jpg
```

## Configuration

All binaries read environment variables from `.env`.

| Variable | Used by | Default | Description |
| --- | --- | --- | --- |
| `LOG_LEVEL` | exchange, relay | `info` | `debug` · `info` · `warn` · `error` |
| `EXCHANGE_ADDR` | burrow | hosted URL | Exchange server address. Accepts `host:port` or a full `https://` URL. |
| `RELAY_ADDR` | burrow | hosted IP:port | Relay TCP address for file transfer |
| `TUNNEL_BIND` | relay | `:8082` | Address the HTTP tunnel server binds to |
| `TUNNEL_PUBLIC_URL` | relay | `http://localhost:8082` | Public base URL embedded in QR codes |

## Project structure

```text
main.go         CLI entrypoint: send, receive, receive-web
cmd/
  exchange/     HTTP + WebSocket exchange server
  relay/        TCP file relay + HTTP web-upload tunnel
internal/
  client/       Burrow client (exchange protocol, file transfer)
  envfile/      Loads the -env .env file into the process environment
  exchange/     Session management, PAKE coordination
  logging/      Shared zerolog console logger + LOG_LEVEL handling
  nameplate/    Random human-readable code generation
  pake/         X25519 ECDH + HKDF key derivation
  progress/     Terminal progress bar (wraps schollz/progressbar)
  qr/           QR code terminal rendering
  relay/        TCP splice + web tunnel hub
  transport/    Exchange HTTP handlers
  tunnel/       Tunnel client (receiver side of web-upload)
  webupload/    Upload HTTP handler + embedded browser UI
```

Container images are built with [ko](https://ko.build) straight from the `cmd/`
packages — no Dockerfile. Config lives in `.ko.yaml`; see `task image` / `task publish`.
