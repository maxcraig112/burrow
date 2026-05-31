# Building from source & local dev

## Prerequisites

- Go 1.22+
- `make` (optional)

## Build

```bash
# All three binaries into bin/
make

# Or individually
make exchange
make relay
make burrow
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
cmd/
  exchange/     HTTP + WebSocket exchange server
  relay/        TCP file relay + HTTP web-upload tunnel
  burrow/       CLI: send, receive, receive-web
internal/
  client/       Burrow client (exchange protocol, file transfer)
  exchange/     Session management, PAKE coordination
  nameplate/    Random human-readable code generation
  pake/         X25519 ECDH + HKDF key derivation
  progress/     Terminal progress bar
  qr/           QR code terminal rendering
  relay/        TCP splice + web tunnel hub
  transport/    Exchange HTTP handlers
  tunnel/       Tunnel client (receiver side of web-upload)
  webupload/    Upload HTTP handler + embedded browser UI
deploy/         Dockerfiles for exchange and relay
terraform/      GCP infrastructure (Cloud Run + Compute Engine)
scripts/        Build and push helper
```
