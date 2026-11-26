# Roadmap

The order of work, from today to a public site. Each step is one pull request,
or a short series, that can ship on its own. [architecture.md](architecture.md)
describes the target and [decisions.md](decisions.md) the reasons.

A step is done when its "done when" lines are true on `main`. The pull request
that completes a step updates this page.

Every step also keeps the other pages true. Whatever it makes wrong, it fixes
in the same pull request, and in particular:

- `AGENTS.md`: the Proof section whenever a step adds something that runs (how
  to run it against throwaway data, and which commands are off limits), and the
  risk paths whenever files move.
- `docs/security/threat-model.md`: whenever a step adds a database, an API, a
  server or a deployment.
- `docs/architecture.md`: the state of each part, from planned to exists.

## Where we are

| Done                                                                                                                                                    |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository, licence, contribution guide, CI, review rules, security policy                                                                              |
| Step 1: the `swisscoverage` program in Go, with its `parse` command and the parser for OFCOM's file, tested on real samples                             |
| Step 2: the database, its schema, and the `migrate` and `ingest` commands, which load the archive and record what changed from one snapshot to the next |
| Step 3: the public API, read-only, served by the `serve` command, and its contract in `openapi.json`                                                    |
| Daily archive of OFCOM's file, running since 23 November 2026 (`swisscoverage-data`), with an index of its snapshots                                    |
| Architecture and decisions written down                                                                                                                 |

In progress: [step 4](#step-4-website-and-map-in-progress), the website and
the map. Its foundation is in `web/`.

Waiting on someone else: a Freedom of Information request for OFCOM's detailed
antenna database was filed on 24 November 2026. Version 1.0 depends on the
answer. See [data-sources.md](data-sources.md).

## Version 0.1: the map is online

The public OFCOM dataset on a map, with filters, behind a public API.

### Step 1. Go foundation (done)

The parser moved to Go, and the repository got its Go toolchain.

- **Added.**
  - `backend/` with the `swisscoverage` program and its first command,
    `parse`: it reads a source file and prints the number of stations, the
    counts per operator, kind and technology, and the warnings. With `-json`
    it prints every station as JSON.
  - The parser in `backend/internal/ofcom`, with the tests of the TypeScript
    parser ported one for one, and tests for what is new now that the parser
    takes the bytes of a file.
  - The sample in `backend/internal/ofcom/testdata/`, Go's directory for test
    data, with a golden file: what the TypeScript parser returned for it.
  - CI: a `backend` job (formatting, `go vet`, `go test`) and a `docs` job
    (Biome). The `check` job gathers them and stays the only required check,
    so branch protection did not change.
- **Proved, once, before the TypeScript parser was removed.** Both parsers
  read the file published on 24 November 2026, with 22,761 sites, and a copy
  of it altered to raise every kind of warning. Their outputs were identical,
  byte for byte, on both.
- **Settled.**
  - Go 1.27.4, the `go` line of `backend/go.mod`, which CI reads.
  - The standard library only, including for the command line.
  - The parser takes the bytes of a file and returns stations and warnings.
    It keeps the names the TypeScript parser gave to its fields.
  - Biome stays at the root, run with Bun, so that Markdown, JSON and YAML
    keep being checked until `web/` brings TypeScript back in step 4. Node is
    no longer needed.
- **Done when.** Both are true:
  - `go test ./...` passes in `backend/`, including the comparison with the
    golden file.
  - `swisscoverage parse` on the day's public file prints about 23,000
    stations and no warning.

### Step 2. Database and import (done)

- **Added.**
  - The schema and its first migration: `snapshot`, `station`,
    `station_state`.
  - The `migrate` and `ingest` commands.
  - Docker Compose with PostGIS, and tests that run against a real database,
    each test in a database of its own.
  - `backend/internal/archive`, which reads the archive's index and files and
    checks them.
  - In the archive repository, the index of snapshots, `snapshots.ndjson`:
    one line per snapshot with its publication time, its checksum, its number
    of sites and the name of its original file. It is rebuilt from the
    repository's history at each run, which also listed the snapshots archived
    before it existed.
- **How the import reads the archive.** It reads the index, downloads each
  original file it has not loaded yet from the archive's releases, verifies its
  checksum, parses it, compares it with the previous snapshot and writes the
  differences.
- **Settled.** `pgx` as the driver and `sqlc` as the generator
  ([decision 7](decisions.md)); migrations as plain SQL files applied by a
  runner of our own ([decision 20](decisions.md)); how history is stored
  ([decision 18](decisions.md)); what the import refuses
  ([decision 19](decisions.md)).
- **Done when.** All are true:
  - Against the Compose database, `ingest` loads every snapshot of the index,
    in order.
  - Running it again changes nothing.
  - Tests cover a site that appears, one that disappears and one that changes,
    and a query returns each of them between two snapshots.

### Step 3. API (done)

- **Added.**
  - The `serve` command, which serves the public API under `/v1`, read-only,
    with these endpoints:
    1. `GET /v1/stations`: every current station in one cacheable GeoJSON
       FeatureCollection, which is what the map loads
       ([decision 12](decisions.md));
    2. `GET /v1/stations/search`: the stations inside a rectangle or around a
       point, filtered by operator, kind, power class and technology;
    3. `GET /v1/stations/{id}`: one station, with its history
       ([decision 22](decisions.md) for the identifier);
    4. `GET /v1/stations/counts`: counts per operator, technology, kind and
       power class;
    5. `GET /v1/snapshots`: the list of snapshots.
  - The `openapi` command, the committed `openapi.json`, and a test, run in
    CI, that fails when the two differ.
  - Validation of every parameter, unknown ones included; ETag and
    `Cache-Control` headers, with 304 for a cache that revalidates
    ([decision 23](decisions.md)); pages of at most 1,000 items for every list
    except the first; gzip; CORS for any origin.
  - Read-only database connections for the API, with a time limit on each
    statement and on each request.
  - A second migration: spatial indexes on the positions. Without them, a
    search around a point within 50 km took 46 seconds instead of 0.2.
- **Settled.** Huma as the OpenAPI library ([decision 21](decisions.md)); a
  station's identifier ([decision 22](decisions.md)); how responses are
  cached ([decision 23](decisions.md)).
- **Measured.** On the snapshot of 24 November 2026, 22,761 sites: the list of
  every station is 5.6 MB, and 670 KB with gzip.
- **Done when.** Both are true:
  - Each endpoint above answers under `curl` against the Compose database
    loaded from the archive.
  - Each appears in the committed `openapi.json`.

### Step 4. Website and map (in progress)

- **Adds.** `web/`: the map with filters by operator, technology, kind and
  power class; a search by station label; the pages; the four languages; types
  generated from `openapi.json`; a `web` job in CI.
- **In pull requests, in this order.**
  1. The foundation (done): Astro and TypeScript on Bun, pages rendered on
     a server, the home page in four languages from the API,
     types generated from `openapi.json`, the `web` job in CI.
  2. The map, with React and MapLibre GL: every station from
     `GET /v1/stations`, grouped when zoomed
     out, the base map with its attribution, a sheet per station, the
     visitor's position on request, a layout for phones.
  3. Filters and the search by label, kept in the address so a view can be
     shared.
  4. The search by address, once its service is in
     [data-sources.md](data-sources.md).
  5. The pages: a page per station, the other pages, every text in four
     languages.
- **Decides.**
  - A page per station, or a map only: a page per station, rendered on the
    server ([decision 24](decisions.md)).
  - The base map.
  - Search by address: which service turns an address into coordinates. It
    must be added to [data-sources.md](data-sources.md) before any code calls
    it. Until then the search covers station labels only.
- **Done when.** The site runs locally against the local API, shows every
  station of the day, filters them, and is usable on a phone-sized screen.

### Step 5. Going online

- **Adds.** The two images and their build in CI; the Helm chart, checked in
  CI; the cluster; the database with backups; DNS at Cloudflare; the protected
  deployment environment.
- **Decides.** The hostnames; the number of nodes; what Cloudflare caches, and
  whether it connects as a proxy or as a tunnel.
- **Done when.**
  - The site's public address, chosen in this step, serves the map from the
    cluster.
  - The daily import runs there on its own.
  - A deployment and a rollback have each been done once on purpose.

## Version 0.2: history and assistants

### Step 6. MCP server

- **Adds.** The `mcp` command, with a client generated from `openapi.json`;
  the test that fails when `internal/mcp` imports the API package, the store
  or a database driver; its deployment, without database credentials.
- **Done when.** An assistant connected to the server over Streamable HTTP
  answers "which sites are within one kilometre of these coordinates, and from
  which operators" from live data.

### Step 7. Visible history

- **Adds.** Endpoints for what was added, removed or changed between two dates;
  the deployment map on the website.
- **Done when.** The site shows, for any period since the archive started, the
  sites that appeared, disappeared or changed.

### Step 8. Statistics and downloads

- **Adds.** Charts per operator and technology over time; downloads of the
  current data and of each chart's series, in open formats.
- **Decides.** The source of administrative boundaries, needed for figures per
  canton or municipality. It must be added to
  [data-sources.md](data-sources.md) before any code uses it.
- **Done when.** Each chart's series can be downloaded, every value with the
  snapshot it comes from.

## Version 1.0: the detailed data

Depends on OFCOM's answer to the Freedom of Information request.

- Model and import for antennas and cells: frequency bands, directions, powers
  where granted, heights, commissioning dates.
- Antenna beams on the map, a sheet per antenna, filters by frequency band.
- Vector tiles, since one response can no longer carry everything.
- The fields of the new source documented in
  [data-sources.md](data-sources.md).

## Later

Not planned in detail, and not before 1.0.

- Estimated coverage, computed with the terrain. Likely a separate job in
  Python.
- Speed tests contributed by users, which are the first data the project could
  not rebuild from the archive.
- Community features.
