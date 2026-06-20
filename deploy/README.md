# Deploy NexusRouter

NexusRouter can run as a local binary, a single Docker container with SQLite, or a
container plus Postgres for team/VPS deployments.

## Quick Start (Pre-built Docker Image)

Pull and run the latest public image from GitHub Container Registry:

```bash
docker pull ghcr.io/ardhycrjr24/NexusRouter:latest

# Run with docker compose (recommended)
docker compose up -d
```

Or run directly with Docker:

```bash
docker run -d \
  --name NexusRouter \
  -p 20180:20180 \
  -v NexusRouter-data:/data \
  ghcr.io/ardhycrjr24/NexusRouter:latest
```

Available tags:
- `ghcr.io/ardhycrjr24/NexusRouter:latest` — latest stable from `main`
- `ghcr.io/ardhycrjr24/NexusRouter:1.2.3` — specific version
- `ghcr.io/ardhycrjr24/NexusRouter:sha-abc1234` — pinned to a commit

## Local Development (One-Liner)

Run this single command — it clones, installs deps, and starts everything:

```bash
curl -fsSL https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/scripts/quickstart.sh | bash
```

No `.env`, no config, no manual steps. It will:
- Check Go 1.24+ and Node.js 20+ are installed
- Clone the repo to `~/NexusRouter` (or use existing checkout)
- Install frontend dependencies (npm ci)
- Download Go modules
- Start backend (:20180) and dashboard (:5180) with hot reload

Dashboard: http://localhost:5180 (password: `NexusRouter`)

> **Already have the repo?** Just run `make setup` from the project root.

## Local Install (Binary)

Build and install the binary + dashboard assets system-wide:

```bash
curl -fsSL https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/scripts/install.sh | bash
NexusRouter
```

If you prefer Docker and do not want Go/Node.js on the machine:

```bash
curl -fsSL https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/scripts/install.sh | bash -s -- --docker
```

## VPS Deployment Guide

### Option 1: VPS with Docker Compose (SQLite - Default)

This is the simplest way to get NexusRouter running on a clean VPS (Ubuntu/Debian).

**1. Clone the repository**
```bash
git clone https://github.com/ardhycrjr24/NexusRouter.git
cd NexusRouter
```

**2. Configure Environment Variables**
```bash
cp .env.example .env
```
Open `.env` with your favorite editor (e.g., `nano .env`). You **must** generate a secure 32-byte master key and set it as `NexusRouter_MASTER_KEY`:
```bash
# Generate a key locally:
openssl rand -base64 32
```
Paste the generated key into your `.env` file.

**3. Start the Deployment**
```bash
docker compose up -d --build
```

**4. View Logs**
```bash
docker compose logs -f NexusRouter
```

**5. Access the Dashboard**
By default, NexusRouter will be available at `http://YOUR_VPS_IP:20180`. It is highly recommended to put NexusRouter behind a reverse proxy like **Nginx**, **Caddy**, or **Traefik** to secure it with HTTPS and a custom domain.

### Option 2: VPS with Postgres

If you are deploying for a team or expecting high traffic, you can use Postgres instead of SQLite.

**1. Prepare `.env`**
Follow the steps above to create your `.env` file, but make sure you also set the database password:
```env
POSTGRES_PASSWORD=your_secure_postgres_password
```

**2. Start the Deployment**
Use the override compose file to start both NexusRouter and Postgres:
```bash
docker compose -f compose.yaml -f compose.postgres.yaml up -d --build
```
*Note: The app container still stores runtime secrets and generated files in a Docker volume mounted at `/data`, while request/account data is stored in the Postgres database.*

## Coolify Deployment Guide

Deploying NexusRouter on [Coolify](https://coolify.io/) is highly recommended as it automates SSL/TLS certificates and reverse proxy configuration.

### Deployment Steps

1. **Create a New Resource**: In your Coolify dashboard, create a new resource and select **Project** -> **Environment** -> **Add New Resource**.
2. **Select Source**: Choose **Git Repository** (Public) and enter:
   - **Repository URL**: `https://github.com/ardhycrjr24/NexusRouter`
   - **Branch**: `main`
3. **Build Pack**: Select **Docker Compose** as the build pack. Pastikan Anda mengatur **Docker Compose Location** (atau *Compose file path*) ke `compose.yaml` karena Coolify mungkin mencari `docker-compose.yaml` secara default.
4. **Configuration**:
   - **Domains**: Enter your custom domain (e.g., `https://NexusRouter.yourdomain.com`). Coolify will automatically map this to the exposed port.
   - **Port**: Make sure the container port is set to `20180` (this is the port NexusRouter listens on inside the container).
5. **Environment Variables**:
   Navigate to the Environment Variables tab in Coolify and add the following variables (Switch to Developer view to edit as text):
   ```env
   NexusRouter_SERVER__HOST=0.0.0.0
   NexusRouter_SERVER__PORT=20180
   NexusRouter_SECURITY__BIND_LOOPBACK_ONLY=false
   # Generate a 32-byte base64 key locally and paste it here:
   NexusRouter_SECURITY__MASTER_KEY=<your_generated_master_key>
   NexusRouter_LOG__FORMAT=json
   ```
6. **Persistent Storage**:
   NexusRouter needs persistent storage for its SQLite database and runtime secrets. In Coolify, go to the **Storages** tab and create a volume:
   - **Name**: `NexusRouter-data`
   - **Destination**: `/data`
7. **Deploy**: Click the **Deploy** button. Coolify will build the Docker image and start the container.

### Using External/Managed Postgres on Coolify

If you provisioned a Postgres database on Coolify or use an external managed DB, add these additional environment variables to your NexusRouter resource:

```env
NexusRouter_DATABASE__DRIVER=postgres
NexusRouter_DATABASE__DSN=postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require
```
*(Replace `USER`, `PASSWORD`, `HOST`, and `DB` with your Postgres credentials).*

## Updates

```bash
git pull
docker compose up -d --build
```

For the local source installer, rerun the install command.

## Security Notes

- Keep `NexusRouter_SECURITY__MASTER_KEY` stable and backed up.
- Use HTTPS when exposing the dashboard outside localhost.
- The dashboard has session auth, but production deployments should still sit
  behind a reverse proxy, firewall, or platform access control.
- The default dashboard password is `NexusRouter`; change it on first login.
