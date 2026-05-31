# Self-hosting

You can run your own exchange and relay servers on a home server, VPS, Raspberry Pi, or GCP. Docker images are available for amd64 and arm64.

## Option 1: Docker Compose (recommended)

Copy the `docker-compose.yml` from the repo root, edit the `TUNNEL_PUBLIC_URL` to your server's IP or hostname, then start it:

```bash
# Edit TUNNEL_PUBLIC_URL first
docker compose up -d
```

To pull the latest images later:

```bash
docker compose pull && docker compose up -d
```

## Option 2: Bare binaries

Grab the latest `exchange-linux-amd64` and `relay-linux-amd64` from the [releases page](https://github.com/maxcraig112/burrow/releases/latest), then make them executable:

```bash
chmod +x exchange-linux-amd64 relay-linux-amd64
```

Both servers read a `.env` file from the working directory. Create one:

```bash
LOG_LEVEL=info
TUNNEL_PUBLIC_URL=http://<your-server-ip>:8082
```

Then start them (each in its own terminal, or use systemd/screen/tmux):

```bash
./exchange-linux-amd64
./relay-linux-amd64
```


## Option 3: GCP (Cloud Run + Compute Engine)

This runs the exchange server on Cloud Run and the relay on a small Compute Engine VM. Infrastructure is managed with Terraform.

### Prerequisites

- A GCP project with billing enabled
- [Terraform](https://developer.hashicorp.com/terraform) >= 1.5
- [Google Cloud SDK](https://cloud.google.com/sdk) (`gcloud`)
- Docker Hub images already pushed (the CI workflow handles this, or run `./scripts/docker-build.sh` manually first)

### Deploy

```bash
# 1. Fill in your GCP project ID
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform/terraform.tfvars and set project_id

# 2. Deploy
cd terraform
terraform init
terraform apply
```

Terraform provisions a Cloud Run service for the exchange server and a Compute Engine VM for the relay, opens the required firewall ports, and wires up a static IP for the relay.

### Get the addresses

After `terraform apply` finishes:

```bash
terraform output burrow_env
```

This prints the `EXCHANGE_ADDR` and `RELAY_ADDR` values to paste into `burrow config`:

```bash
burrow config https://<cloud-run-url> <relay-ip>:9090
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

The relay needs three ports reachable by all clients:

| Port | Protocol | Used for |
| --- | --- | --- |
| 8080 | TCP | Exchange server (HTTP/WebSocket) |
| 9090 | TCP | Relay file transfer |
| 8082 | TCP | Web upload tunnel (`receive-web`) |

### Tailscale (recommended)

If everyone on your Burrow setup is on the same [Tailscale](https://tailscale.com) network, you don't need to open any ports. Install Tailscale on the server, get its Tailscale IP, then:

```bash
burrow config http://100.x.x.x:8080 100.x.x.x:9090
```

Use the same Tailscale IP in `TUNNEL_PUBLIC_URL` in `docker-compose.yml` (or your `.env`).
