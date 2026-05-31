# Clean Demo Database

Use the demo database generator when a demo site must start without customer
history. The generated SQLite file is separate from every production database.

It contains:

- one generic company and one demo division
- eight snooker tables with the same rates: 150 normal and 100 practice
- relay mappings 1 through 8
- one admin user with full permissions
- baseline POS and system settings
- basic drinks, food, snacks, and snooker products
- opening stock and matching stock-ledger entries for stock-managed products

It does not contain:

- customer bills
- historical sales
- product-receipt bills
- open tables
- customer names, addresses, or credentials

## Generate locally

Run from the `backend` directory. Set passwords locally and do not commit them.

```bash
export DEMO_ADMIN_PASSWORD='set-a-password-before-running'
export DEMO_CLOSE_TABLE_PASSWORD='set-a-close-table-password'
export DEMO_EDIT_REPORT_PASSWORD='set-an-edit-report-password'

go run ./cmd/seed-demo \
  --output ../tmp/demo-seed/snooker.db \
  --force
```

The generated file is ignored by Git:

```text
tmp/demo-seed/snooker.db
```

If an initial password is saved locally for handoff, keep that file under
`tmp/demo-seed`, never commit it, and delete it after the demo account password
has been changed.

Upload the generated database from the local machine:

```bash
scp tmp/demo-seed/snooker.db root@DEMO_SERVER_IP:/tmp/snooker-demo-clean.db
```

## Install on the four-site demo server

Stop the demo stack before replacing database files. Keep a timestamped backup.

```bash
cd /opt/snooker-demo-next
STAMP=$(date +%F_%H%M%S)
mkdir -p "/opt/snooker-backups/demo-clean-$STAMP"
for site in demo1 demo2 demo3 demo4; do
  mkdir -p "/opt/snooker-backups/demo-clean-$STAMP/$site"
  cp -a "data/$site/snooker.db" "/opt/snooker-backups/demo-clean-$STAMP/$site/snooker.db"
done

docker compose -f docker-compose.demo.yml down

for site in demo1 demo2 demo3 demo4; do
  cp /tmp/snooker-demo-clean.db "data/$site/snooker.db"
  mkdir -p "data/uploads/$site"
  cp backend/uploads/YSdemo.png "data/uploads/$site/YSdemo.png"
  cp backend/uploads/LOGO_Final_Stoke2.png "data/uploads/$site/LOGO_Final_Stoke2.png"
done

docker compose -f docker-compose.demo.yml up -d --no-build
docker compose -f docker-compose.demo.yml ps
```

Each site receives its own copy. Do not mount one SQLite file into multiple
backend containers.
