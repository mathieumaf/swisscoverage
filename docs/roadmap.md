# Roadmap

The order of work, from today to a public site. Each step is one pull request,
or a short series, that can ship on its own. [architecture.md](architecture.md)
describes the target and [decisions.md](decisions.md) the reasons.

A step is done when its "done when" line is true on `main`. Update this page in
the pull request that completes a step.

## Where we are

| Done                                                                                 |
| ------------------------------------------------------------------------------------ |
| Repository, licence, contribution guide, CI, review rules, security policy           |
| Parser for OFCOM's file, in TypeScript (`packages/core`), tested on real samples     |
| Daily archive of OFCOM's file, running since 23 November 2026 (`swisscoverage-data`) |
| Architecture and decisions written down                                              |

Waiting on someone else: a Freedom of Information request for OFCOM's detailed
antenna database was filed on 24 November 2026. Version 1.0 depends on the
answer. See [data-sources.md](data-sources.md).

## Version 0.1: the map is online

The public OFCOM dataset on a map, with filters, behind a public API.

### Step 1. Go foundation

- **Adds.** `backend/` as a Go module with the `swisscoverage` program; the
  parser ported to `backend/internal/ofcom` with the same tests and the same
  fixture; a Go job in CI.
- **Removes.** `packages/core` and the TypeScript tooling at the root, once the
  port passes. The TypeScript tooling comes back with `web/` in step 4.
- **Updates.** `AGENTS.md` (proof commands, review rules, risk paths),
  `CONTRIBUTING.md`, `docs/data-sources.md` (the parser's path).
- **Done when.** The Go parser reads today's public file with the same result
  as the TypeScript one: the same number of stations, zero warnings, the same
  counts per operator, kind and technology.

### Step 2. Database and import

- **Adds.** The schema and its migrations (`snapshot`, `station`,
  `station_state`); the `migrate` and `ingest` commands; Docker Compose with
  PostGIS; tests that run against a real database.
- **Decides.** The SQL generator, the driver and the migration tool.
- **Done when.** `ingest` loads every snapshot of the archive in order, running
  it twice changes nothing, and a query returns what changed between two days.

### Step 3. API

- **Adds.** The `serve` command; endpoints for the stations of an area with
  filters, one station with its history, statistics, and the list of
  snapshots; `openapi.json` and the CI check that keeps it in step with the
  code; cache headers.
- **Decides.** The OpenAPI library.
- **Done when.** The website's needs for version 0.1 can be met with `curl`
  alone, and the contract is committed.

### Step 4. Website and map

- **Adds.** `web/`: the map with filters by operator, technology, kind and
  power class; a search by address; the pages; the four languages; types
  generated from `openapi.json`.
- **Decides.** Pages per station or map only; the base map.
- **Done when.** The site runs locally against the local API, with every
  station of the day on the map, and works on a phone.

### Step 5. Going online

- **Adds.** The two images and their build in CI; the Helm chart, checked in
  CI; the cluster; the database with backups; DNS at Cloudflare; the protected
  deployment environment.
- **Decides.** The API's hostname; the number of nodes; what Cloudflare caches,
  and whether it is a proxy or a tunnel.
- **Done when.** `swisscoverage.ch` serves the map from the cluster, the daily
  import runs there on its own, and a deployment and a rollback have each been
  done once on purpose.

## Version 0.2: history and assistants

### Step 6. MCP server

- **Adds.** The `mcp` command: tools that answer questions by calling the
  public API; its deployment.
- **Done when.** An assistant connected to the server answers "which sites are
  within one kilometre of this address, and from which operators" from live
  data.

### Step 7. Visible history

- **Adds.** Endpoints for what was added, removed or changed between two dates;
  the deployment map on the website.
- **Done when.** The site shows, for any period since the archive started, the
  sites that appeared, disappeared or changed.

### Step 8. Statistics and downloads

- **Adds.** Charts per operator, technology and canton over time; downloads of
  the current data in open formats.
- **Done when.** Every figure on the site can be downloaded and traced to a
  snapshot.

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
