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

| Done                                                                                                                        |
| --------------------------------------------------------------------------------------------------------------------------- |
| Repository, licence, contribution guide, CI, review rules, security policy                                                  |
| Step 1: the `swisscoverage` program in Go, with its `parse` command and the parser for OFCOM's file, tested on real samples |
| Daily archive of OFCOM's file, running since 23 November 2026 (`swisscoverage-data`)                                        |
| Architecture and decisions written down                                                                                     |

Next: [step 2](#step-2-database-and-import), the database and the import.

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

### Step 2. Database and import

- **Adds.**
  - The schema and its migrations: `snapshot`, `station`, `station_state`.
  - The `migrate` and `ingest` commands.
  - Docker Compose with PostGIS, and tests that run against a real database.
  - In the archive repository, an index of snapshots: one line per snapshot
    with its publication time, its checksum, its number of sites and the name
    of its original file. The snapshots already archived are added to it from
    the repository's history.
- **How the import reads the archive.** It reads the index, downloads each
  original file it has not loaded yet from the archive's releases, verifies its
  checksum, parses it, compares it with the previous snapshot and writes the
  differences.
- **Decides.** The SQL generator, the driver and the migration tool.
- **Done when.**
  - Against the Compose database, `ingest` loads every snapshot of the index,
    in order.
  - Running it again changes nothing.
  - Tests cover a site that appears, one that disappears and one that changes,
    and a query returns each of them between two snapshots.

### Step 3. API

- **Adds.**
  - The `serve` command, with these endpoints:
    1. every current station in one cacheable response, which is what the map
       loads ([decision 12](decisions.md));
    2. the stations inside a rectangle or around a point, with filters;
    3. one station, with its history;
    4. counts per operator, technology, kind and power class;
    5. the list of snapshots.
  - The `openapi` command, the committed `openapi.json`, and the CI check that
    fails when the two differ.
  - Validation of every parameter, cache headers, and a limit on the size of
    every response except the first, which the dataset itself bounds.
- **Decides.** The OpenAPI library.
- **Done when.**
  - Each endpoint above answers under `curl` against the Compose database
    loaded from the archive.
  - Each appears in the committed `openapi.json`.

### Step 4. Website and map

- **Adds.** `web/`: the map with filters by operator, technology, kind and
  power class; a search by station label; the pages; the four languages; types
  generated from `openapi.json`; a `web` job in CI.
- **Decides.**
  - A page per station, or a map only.
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
