# Burrow

Encrypted peer-to-peer file transfer over a relay, inspired by [magic-wormhole](https://github.com/magic-wormhole/magic-wormhole) — a Python package. Since a gopher's home is called a burrow, this is the Go take on the same idea.

Two machines share a short human-readable code; the actual file data never touches any server in plaintext.

## What Burrow adds

| Feature | magic-wormhole | Burrow |
| --- | --- | --- |
| CLI send / receive | yes | yes |
| Browser upload (`receive-web`) | no | yes — receiver prints a QR code; anyone who scans it gets a drag-and-drop upload page |
| Written in | Python | Go (single static binary, no runtime) |
| Self-hostable infra | no | yes — Terraform for GCP (Cloud Run + Compute Engine) |

## How it works

```text
┌──────────┐   WebSocket/PAKE   ┌──────────────┐
│  Sender  │ ─────────────────► │   Exchange   │
└──────────┘   HTTP/PAKE        │   Server     │
┌──────────┐ ◄───────────────── └──────────────┘
│ Receiver │
└──────────┘
      │
      │  TCP (AES-256-GCM chunks)
      ▼
┌──────────────┐
│    Relay     │  ── TCP :9090  (file transfer)
│    Server    │  ── HTTP :8082 (web-upload tunnel)
└──────────────┘
```

1. **Exchange server** — coordinates sessions. Sender and receiver each send their X25519 public key; the server relays them so both sides derive the same shared secret (ECDH + HKDF).
2. **Relay server** — a dumb TCP splice. Once both sides connect with a matching token, bytes flow directly between them; the relay never sees decrypted data. It also hosts the HTTP tunnel for `receive-web`.
3. **Wormhole CLI** — the tool you actually run. Encrypts files with AES-256-GCM and streams them through the relay.

## Prerequisites

- Go 1.22+
- `make` (optional, for Makefile shortcuts)
- Running exchange and relay servers (see [Server setup](#server-setup-local))

## Building

```bash
# Build all three binaries into bin/
make

# Or individually
make exchange
make relay
make wormhole
```

## Server setup (local)

Both servers read `.env` from the working directory. The defaults work out of the box:

```bash
# Terminal 1 — exchange server on :8080
./bin/exchange

# Terminal 2 — relay server on :9090 (TCP) and :8082 (HTTP tunnel)
./bin/relay
```

Use `-env /path/to/file` to load a different env file.

## CLI usage

> The wormhole CLI reads `EXCHANGE_ADDR` and `RELAY_ADDR` from `.env`.

### Send a file

```bash
./bin/wormhole send photo.jpg
```

```text
Connecting to exchange server...

Code: swift-copper-leaps
On the other machine run:

    wormhole receive swift-copper-leaps

Waiting for receiver...
Receiver connected. Sending photo.jpg (4.2 MB)...
  [████████████████████████████] 100.0%  4.2 MB / 4.2 MB  12.3 MB/s  (0s)
Done!
```

### Receive a file

```bash
./bin/wormhole receive swift-copper-leaps
```

```text
Connecting to exchange server...
Connecting to relay...
Receiving photo.jpg (4.2 MB)...
  [████████████████████████████] 100.0%  4.2 MB / 4.2 MB  10.8 MB/s  (0s)
Saved to /home/user/photo.jpg
Done!
```

The file is saved to the current working directory.

### Receive via browser (web upload)

```bash
./bin/wormhole receive-web
```

```text
Registering tunnel...

Code: misty-harbor-glides
Scan the QR code or open in a browser:
  http://relay-host:8082/t/misty-harbor-glides/

[QR CODE]

Waiting for uploads — press Ctrl+C to stop.
```

Anyone who scans the QR code gets a drag-and-drop upload page. Files are saved to the current working directory. Multiple uploads are accepted until Ctrl+C.

> For internet access, set `TUNNEL_PUBLIC_URL` in the relay's environment to its public address (e.g. `http://1.2.3.4:8082`). See [Deployment](#deployment).

## Configuration

All binaries load environment variables from `.env` in the working directory.

| Variable | Used by | Default | Description |
| --- | --- | --- | --- |
| `LOG_LEVEL` | exchange, relay | `info` | `debug` · `info` · `warn` · `error` |
| `EXCHANGE_ADDR` | wormhole | `localhost:8080` | Exchange server address. Accepts a full `https://` URL for TLS deployments. |
| `RELAY_ADDR` | wormhole | `localhost:9090` | Relay TCP address for file transfer |
| `TUNNEL_BIND` | relay | `:8082` | Address the HTTP tunnel server binds to |
| `TUNNEL_PUBLIC_URL` | relay | `http://localhost:8082` | Public base URL embedded in QR codes |

## Deployment

Infrastructure is managed with Terraform. The exchange server runs on **Cloud Run** (HTTP + WebSocket, scales to zero); the relay runs on a **Compute Engine** VM because it needs a raw TCP port.

```bash
# 1. Fill in your GCP project ID
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
$EDITOR terraform/terraform.tfvars

# 2. Bootstrap the registry and static IP first
cd terraform
terraform init
terraform apply \
  -target=google_artifact_registry_repository.burrow \
  -target=google_compute_address.relay

# 3. Build and push Docker images
cd ..
./scripts/docker-build.sh YOUR_PROJECT_ID

# 4. Deploy everything
cd terraform && terraform apply
```

After apply, print the wormhole `.env` values:

```bash
terraform output wormhole_env
```

## Project structure

```text
cmd/
  exchange/     HTTP + WebSocket exchange server
  relay/        TCP file relay + HTTP web-upload tunnel
  wormhole/     CLI: send, receive, receive-web
internal/
  client/       Wormhole client (exchange protocol, file transfer)
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

## Security notes

- Keys are exchanged via **X25519 ECDH**; the shared secret is derived with **HKDF-SHA256**.
- File data is encrypted with **AES-256-GCM** before leaving the sender's machine.
- The relay handles only opaque ciphertext — it cannot read transferred files.
- The exchange server handles only public keys — it never sees file data.
