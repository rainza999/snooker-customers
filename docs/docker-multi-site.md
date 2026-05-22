# Docker Multi-Site Deploy Notes

## DB placement

Keep each site's SQLite DB and license folder separate:

```text
data/
  demo1/snooker.db        # ch-chang on the current production layout
  demo1/license/license.json
  neverdie/snooker.db
  neverdie/license/license.json
  om/snooker.db           # use when om runs on the same server as other sites
  om/license/license.json
  site4/snooker.db
  site4/license/license.json
  uploads/
    neverdie/
    om/
    site4/
```

The `data/` folder is ignored by git, so real customer DB files and license files should stay only on the server.

## License migration before recreating containers

Old production containers may store `license.json` inside the container at `/app/license.json`. Copy it out before `docker compose up --build` recreates the container:

```bash
mkdir -p data/demo1/license data/neverdie/license data/om/license
docker cp golang-snooker-demo1:/app/license.json data/demo1/license/license.json
docker cp golang-snooker-neverdie:/app/license.json data/neverdie/license/license.json
```

If a copied license was already activated, keep its existing machine ID by adding it to `.env`:

```env
MACHINE_ID_DEMO1=paste-existing-demo1-machine-id
MACHINE_ID_NEVERDIE=paste-existing-neverdie-machine-id
MACHINE_ID_OM=paste-existing-om-machine-id
```

Only use `MACHINE_ID_SEED_*` for new activations or when you intentionally want to issue a new license.

## Existing layouts

For the current ch-chang + neverdie server:

```bash
mkdir -p data/demo1 data/neverdie data/uploads/neverdie
# copy ch-chang DB to data/demo1/snooker.db
# copy neverdie DB to data/neverdie/snooker.db
docker compose up -d --build site golang-demo1 golang-neverdie
```

For the current om-only server that still stores the om DB in the `demo1` slot:

```bash
mkdir -p data/demo1 data/uploads
# copy om DB to data/demo1/snooker.db
docker compose up -d --build site golang-demo1
```

Those commands do not need a `.env` file.

## One server with ch-chang, neverdie, om, and a fourth site

Create `.env` on that server:

```env
UPLOADS_DEMO1_PATH=./data/uploads/demo1
UPLOAD_ROOT_CH_CHANG=/srv/uploads/demo1

BACKEND_OM=golang-om
UPLOAD_ROOT_OM=/srv/uploads/om

SITE4_HOST=site4.yssnooker.com
BACKEND_SITE4=golang-site4
UPLOAD_ROOT_SITE4=/srv/uploads/site4
```

Then place DB files:

```text
data/demo1/snooker.db
data/neverdie/snooker.db
data/om/snooker.db
data/site4/snooker.db
data/uploads/demo1/
data/uploads/neverdie/
data/uploads/om/
data/uploads/site4/
```

Start the needed profiles:

```bash
docker compose --profile om --profile site4 up -d --build site golang-demo1 golang-neverdie golang-om golang-site4
```

Each backend has its own stable `MACHINE_ID_SEED` and its own mounted license folder, so license files stay separated per site.
