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

On each machine using Burrow, create a `.env` in the directory you run `burrow` from:

```bash
EXCHANGE_ADDR=http://<your-server-ip>:8080
RELAY_ADDR=<your-server-ip>:9090
```

## Networking

The relay needs two ports reachable by all clients:

| Port | Protocol | Used for |
| --- | --- | --- |
| 8080 | TCP | Exchange server (HTTP/WebSocket) |
| 9090 | TCP | Relay file transfer |
| 8082 | TCP | Web upload tunnel (`receive-web`) |

### Option 1: Tailscale (recommended)

If everyone who uses your Burrow setup is on the same [Tailscale](https://tailscale.com) network, you don't need to open any ports at all. Just install Tailscale on the server, get its Tailscale IP, and use that in your `.env`:

```bash
EXCHANGE_ADDR=http://100.x.x.x:8080
RELAY_ADDR=100.x.x.x:9090
```

Tailscale handles the encrypted tunneling between devices, so the servers don't need to be exposed to the internet.

### Option 2: Open ports

If you want to use Burrow without Tailscale, open the three ports above in your firewall/router. On UFW:

```bash
ufw allow 8080/tcp
ufw allow 9090/tcp
ufw allow 8082/tcp
```

Then use your server's public IP or hostname in the `.env`.

## Running as a service (optional)

To keep the servers running after logout, create systemd units. Example for the exchange server at `/etc/systemd/system/burrow-exchange.service`:

```ini
[Unit]
Description=Burrow exchange server
After=network.target

[Service]
ExecStart=/opt/burrow/exchange-linux-amd64
WorkingDirectory=/opt/burrow
Restart=always
Environment=LOG_LEVEL=info

[Install]
WantedBy=multi-user.target
```

Do the same for `burrow-relay.service`, then:

```bash
systemctl enable --now burrow-exchange burrow-relay
```
