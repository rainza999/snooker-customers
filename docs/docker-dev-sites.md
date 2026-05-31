# Docker Dev Server: Named Sites

This compose file runs the dev customer playground without touching the demo
compose stack:

```text
dev.yssnooker.com        -> golang-dev -> data/dev/snooker.db
rain.yssnooker.com       -> golang-rain -> data/rain/snooker.db
smart-home.yssnooker.com -> golang-smart-home -> data/smart-home/snooker.db
arayasujin.yssnooker.com -> golang-arayasujin -> data/arayasujin/snooker.db
```

Each site has its own SQLite database, uploads folder, license folder, config
folder, stable machine ID seed, and JWT secret. The frontend build is shared.

## DNS

Create A records pointing to the dev server IP:

```text
dev.yssnooker.com
rain.yssnooker.com
smart-home.yssnooker.com
arayasujin.yssnooker.com
```

## Server Preparation

On the dev server:

```bash
cd /opt/snooker-dev
cp docker-compose.dev.env.example .env
nano .env

mkdir -p \
  data/{dev,rain,smart-home,arayasujin}/{license,config} \
  data/uploads/{dev,rain,smart-home,arayasujin} \
  nginx/ssl/live/yssnooker.com nginx/log
```

Generate a different JWT secret for each site and paste the values into `.env`:

```bash
openssl rand -hex 32
```

Place a valid certificate and private key on the server:

```text
nginx/ssl/live/yssnooker.com/fullchain.pem
nginx/ssl/live/yssnooker.com/privkey.pem
```

The certificate must cover all four hostnames above.

## Database Placement

Copy a separate database file for each site:

```text
data/dev/snooker.db
data/rain/snooker.db
data/smart-home/snooker.db
data/arayasujin/snooker.db
```

Do not mount one SQLite file into multiple backend containers.

## Start Or Update

```bash
docker compose -f docker-compose.dev.yml build
docker compose -f docker-compose.dev.yml up -d
docker compose -f docker-compose.dev.yml ps
```

Health check:

```bash
for host in dev.yssnooker.com rain.yssnooker.com smart-home.yssnooker.com arayasujin.yssnooker.com; do
  echo "== $host =="
  curl -sk https://$host/api/health || true
  echo
done
```
