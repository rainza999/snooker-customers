# Docker Demo Server: Four Sites

This compose file runs four isolated demo sites on one server:

```text
demo1.yssnooker.com -> golang-demo1 -> data/demo1/snooker.db
demo2.yssnooker.com -> golang-demo2 -> data/demo2/snooker.db
demo3.yssnooker.com -> golang-demo3 -> data/demo3/snooker.db
demo4.yssnooker.com -> golang-demo4 -> data/demo4/snooker.db
```

Each site has its own SQLite database, uploads, license folder, config folder,
stable machine ID seed, and JWT secret. The frontend build is shared.

## Server preparation

Create four DNS records pointing to the demo server IP:

```text
demo1.yssnooker.com
demo2.yssnooker.com
demo3.yssnooker.com
demo4.yssnooker.com
```

On the server:

```bash
cd /opt/snooker-demo
cp docker-compose.demo.env.example .env
nano .env

mkdir -p \
  data/demo{1,2,3,4}/{license,config} \
  data/uploads/demo{1,2,3,4} \
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

The certificate must cover all four demo subdomains.

## Database placement

Copy a separate database file for each demo site:

```text
data/demo1/snooker.db
data/demo2/snooker.db
data/demo3/snooker.db
data/demo4/snooker.db
```

Do not mount one SQLite file into multiple containers.

For a clean database without customer history, generate the template locally
with `backend/cmd/seed-demo` and copy the resulting SQLite file once per site.
See `docs/demo-clean-database.md`.

## Start or update

```bash
docker compose -f docker-compose.demo.yml build golang-demo1
docker compose -f docker-compose.demo.yml up -d --no-build
docker compose -f docker-compose.demo.yml ps
```

Each site activates its own license because each container mounts a separate
`data/demoN/license` directory.
