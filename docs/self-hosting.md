# Self-hosting

You can run your own exchange and relay servers on any Linux machine — a home server, a VPS, a Raspberry Pi, etc.

## Download the binaries

Grab the latest `exchange-linux-amd64` and `relay-linux-amd64` from the [releases page](https://github.com/maxcraig112/burrow/releases/latest), then make them executable:

```bash
chmod +x exchange-linux-amd64 relay-linux-amd64
```

## Run the servers

Both servers read a `.env` file from the working directory. Create one:

```bash
LOG_LEVEL=info
TUNNEL_PUBLIC_URL=http://<your-server-ip>:8082
```

Then start them (each in its own terminal, or use systemd/screen/tmux):

```bash
# Exchange server — HTTP + WebSocket on :8080
./exchange-linux-amd64

# Relay server — TCP on :9090, HTTP tunnel on :8082
./relay-linux-amd64
```

## Point the CLI at your servers

Run this once on each machine that will use Burrow:

```bash
burrow config http://<your-server-ip>:8080 <your-server-ip>:9090
```

This saves the addresses to `~/.config/burrow/config` (Windows: `%AppData%\burrow\config`) and they're picked up automatically from then on. Check the current config any time with:

```bash
burrow config
```

A local `.env` in the working directory always takes priority over the saved config if you need to override temporarily.

## Networking

The relay needs two ports reachable by all clients:

| Port | Protocol | Used for |
| --- | --- | --- |
| 8080 | TCP | Exchange server (HTTP/WebSocket) |
| 9090 | TCP | Relay file transfer |
| 8082 | TCP | Web upload tunnel (`receive-web`) |

### Option 1: Tailscale (recommended)

If everyone who uses your Burrow setup is on the same [Tailscale](https://tailscale.com) network, you don't need to open any ports at all. Install Tailscale on the server, get its Tailscale IP, and use that when running `burrow config`:

```bash
burrow config http://100.x.x.x:8080 100.x.x.x:9090
```

Tailscale handles the encrypted tunneling between devices so the servers don't need to be exposed to the internet.

### Option 2: Open ports

Open the three ports in your firewall/router. On UFW:

```bash
ufw allow 8080/tcp
ufw allow 9090/tcp
ufw allow 8082/tcp
```

Then use your server's public IP or hostname when running `burrow config`.
