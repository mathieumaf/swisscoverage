# Decisions

Why swisscoverage is built the way it is. Each entry says what was decided,
why, what was rejected, and what would make us change our mind.
[architecture.md](architecture.md) describes the result;
[roadmap.md](roadmap.md) gives the order of work.

To change a decision, open a pull request that edits its entry and the pages
that depend on it. To add one, append it: entries are never renumbered.

Unless noted otherwise, entries were accepted on 24 November 2026.

## Context

Three facts shaped most choices:

- **The data is small and changes once a day.** About 23,000 sites today, and
  on the order of 140,000 cells once the detailed data arrives. No language or
  framework considered here would be limited by the load. What limits a service
  like this one is the database and the cache.
- **The project has one maintainer** and is meant to run for years. What costs
  the least to keep running matters more than what is fastest to write.
- **The project is open source and meant to be reproduced.** Anyone should be
  able to run it locally and to host their own instance.

## Accepted

### 1. The archive is the source of truth

- **Decision.** Every file OFCOM publishes is kept in a separate repository,
  [swisscoverage-data](https://github.com/mathieumaf/swisscoverage-data), both
  as the original and as one site per line. The database is derived from it.
- **Why.** OFCOM replaces its file every day and keeps no history. A day that
  is not archived is lost for good. With the archive, a bug in the parser or a
  lost database costs time, never information.
- **Rejected.** Keeping history only in the database: a parsing error would
  then be permanent. Keeping the archive in this repository: every clone would
  download years of data.
- **Revisit when.** The archive outgrows what a git repository handles well.

### 2. One public API, and every client goes through it

- **Decision.** The website, the MCP server and third parties read the same
  public HTTP API. Nothing else reaches the data.
- **Why.** Reading the data without opening the map is a goal of the project,
  not a side effect. If the map needs something the API lacks, third parties
  lack it too, and the gap is visible at once.
- **Rejected.** Letting the website query the database directly: quicker to
  build, but the API would then lag behind the map.
- **Revisit when.** Nothing foreseeable.

### 3. Go for the backend

- **Decision.** The API, the daily import and the MCP server are written in Go.
- **Why.** The backend is a long-lived public service run on Kubernetes by one
  person. Go gives a single static binary, few dependencies to follow, and a
  language that promises not to break existing code. The parts that must last
  the longest get the most stable language.
- **Rejected.** TypeScript everywhere: one language for every contributor and
  the fastest route to a first version, but a faster-moving ecosystem to keep
  up with. Python: the best tools for geodata and analysis, which nothing needs
  before coverage modelling, and a heavier runtime.
- **Cost.** The parser, first written in TypeScript, was ported to Go in
  roadmap step 1. The repository has two toolchains.
- **Revisit when.** The priority becomes contributor reach over upkeep, or the
  backend turns out to need analysis libraries that only Python has.

### 4. TypeScript for the website

- **Decision.** The website and the map are written in TypeScript, with Astro
  for the pages and React with MapLibre GL for the map.
- **Why.** The map runs in the browser, so the language is not a choice. The
  site is mostly pages of content in four languages around one very
  interactive map, which is what Astro is built for: little JavaScript sent,
  routing by language included, the map as one isolated component.
- **Rejected.** Next.js: the most widespread, but heavier and harder to host
  oneself. SvelteKit: light and fast, with fewer contributors than React.
  TanStack Start: well typed, and younger than the others.
- **Revisit when.** The whole site becomes one application rather than pages
  around a map. The framework can change without touching the backend.

### 5. Python later, and apart

- **Decision.** No Python until something needs it. Coverage modelling, the
  likely first need, will be a separate job that reads the database and writes
  its results back.
- **Why.** Three languages for one maintainer is one too many, and nothing in
  versions 0.1 or 1.0 calls for Python. A separate job can be added without
  touching the API.
- **Rejected.** A Python backend from the start, see decision 3.
- **Revisit when.** Analysis becomes the core of the project before 1.0.

### 6. PostgreSQL with PostGIS

- **Decision.** One PostgreSQL database with the PostGIS extension.
- **Why.** The questions are spatial ("what is around here", "in this
  municipality") and temporal ("what changed, and when"). PostGIS also does the
  rigorous transformation from the Swiss grid to WGS84, which approximate
  formulas miss by up to three metres near Geneva.
- **Rejected.** Static files: no real API. SQLite or DuckDB: awkward with
  several replicas of the API, and no path to the writes that speed tests will
  need.
- **Revisit when.** Nothing foreseeable.

### 7. SQL written by hand, typed code generated from it

- **Decision.** Queries are written in SQL. A generator turns them into typed
  Go code. No object-relational mapper.
- **Why.** PostGIS is driven in SQL. A layer that hides SQL gets in the way as
  soon as a query becomes spatial, and hand-written queries are easy to review.
- **Rejected.** An object-relational mapper.
- **Confirmed in roadmap step 2.** The driver is `pgx` and the generator is
  `sqlc`. `sqlc` reads the schema from the migrations and needs no database
  to run. It is not a dependency of the program: `go generate` runs a pinned
  version of it, and the code it writes is committed.
- **Revisit when.** Nothing foreseeable.

### 8. A generated API contract, committed to the repository

- **Decision.** The backend prints the API contract from its code
  (`swisscoverage openapi`). The result is committed as `openapi.json`, and CI
  fails when the committed file differs from what the code prints.
- **Why.** The contract is the only thing the website and the MCP server know
  about the backend. Committed, every change to it appears in a pull request's
  diff. The website's types and the MCP server's client are generated from it,
  so a breaking change fails a build instead of production.
- **Rejected.** Writing the contract by hand first: more rigorous, slower for
  one maintainer. GraphQL: more than read-only data needs.
- **Revisit when.** Third parties depend on the API enough to want each change
  reviewed before any code is written.

### 9. The MCP server is a separate client of the API

- **Decision.** The MCP server is its own package and its own deployment. It
  calls the public API over HTTP, through a client generated from
  `openapi.json`, and has no access to the database. Assistants reach it over
  Streamable HTTP. It is written in Go, as the `mcp` command of the backend
  program.
- **Why.** It proves the API is enough: what the MCP server cannot do, no third
  party can. There is one path to the data to secure, not two. And it can be
  replaced without touching anything else.
- **Rejected.** Mounting the MCP tools inside the API on the same queries: one
  service fewer, but a second path to the data to secure and to keep
  consistent.
- **Cost.** One more deployment, from the same image, and one more HTTP round
  trip per question.
- **Guardrail.** Sharing a program with the API must not become sharing its
  access. A test fails the build when the MCP package imports the API package,
  the store or a database driver, and the deployment gives it no database
  credentials.
- **Revisit when.** The Go SDK for MCP proves limiting. Because the server is
  only a client of the API, it can then be rewritten in another language
  without affecting the rest.

### 10. One backend program with several commands

- **Decision.** The Go backend is one program. `serve`, `mcp`, `ingest` and
  `migrate` run in production; `parse` and `openapi` are tools for developers.
  It ships as one image. The website ships as a second image.
- **Why.** One build from one commit for the API, the MCP server and the
  import, instead of three to publish and to keep in step.
- **To know.** Built together does not mean running the same version at every
  moment: two versions overlap during an update and after a rollback. The
  schema and the API therefore stay compatible with the previous version, and
  the API is updated before its clients and rolled back after them.
- **Rejected.** One image per service: finer builds, more to publish.
- **Revisit when.** The image becomes heavy, or one command needs dependencies
  the others should not carry.

### 11. A site is its label and its position

- **Decision.** A site is identified by the pair of its station label and its
  position.
- **Why.** OFCOM publishes no stable identifier. The label alone is not
  unique: one label appears at up to 14 positions. The pair was unique in the
  snapshot of 23 November 2026.
- **Limit.** A site moved by a few metres appears as a removal followed by an
  addition.
- **Revisit when.** A few weeks of archive show how often that happens. Sites
  that moved could then be matched afterwards.

### 12. One payload of stations first, vector tiles later

- **Decision.** Version 0.1 serves all stations to the map in one cached
  response. Vector tiles come with version 1.0.
- **Why.** 23,000 points fit in one response. Filters are then instant, and
  MapLibre clusters the points itself.
- **Rejected for now.** Vector tiles produced by PostGIS: needed once the map
  shows cells and antenna beams, more than version 0.1 requires.
- **Confirmed in roadmap step 3.** The response is a GeoJSON
  FeatureCollection, which MapLibre loads and clusters as it is, with what the
  map filters on and nothing more; the rest is in each station's own
  response. On the snapshot of 24 November 2026, 22,761 sites, it is 5.6 MB,
  and 670 KB with gzip.
- **Revisit when.** The response proves too heavy on a phone.

### 13. The database runs inside the cluster

- **Decision.** PostgreSQL runs in the Kubernetes cluster, managed by the
  CloudNativePG operator, with backups to object storage.
- **Why.** The same Helm chart then works on any cluster, which decision 14
  requires. In November 2026 the intended host listed managed PostgreSQL as
  coming soon, with no information on PostGIS. And since the database can be
  rebuilt from the archive, the risk is low.
- **Rejected.** A managed database: not available. A database on a separate
  machine: a second system to maintain.
- **Revisit when.** A managed PostgreSQL with PostGIS becomes available, or the
  database starts to hold data that cannot be rebuilt.

### 14. Kubernetes with a Helm chart

- **Decision.** Production runs on Kubernetes, described by a Helm chart. The
  public instance is meant for a managed cluster in Switzerland, and the chart
  must also work on a self-managed cluster. Docker Compose is for local work
  and tests only.
- **Why.** Updates without downtime, and a standard deployment that others can
  reproduce.
- **Cost.** More expensive and more moving parts than one server. Given the
  size of the data, load is not what justifies it.
- **Rejected.** One server with Docker Compose: cheaper and simpler, with
  downtime at each update and nothing others can reuse as is.
- **Revisit when.** The cost outweighs the benefit for a project of this size.

### 15. Deployment by GitHub Actions, after approval

- **Decision.** A merge to `main` builds and publishes the images. Deploying
  them runs through a protected GitHub environment and waits for the
  maintainer's approval.
- **Why.** Simple, traceable, and nothing reaches production unattended.
- **Rejected.** A GitOps controller in the cluster (Argo CD, Flux): useful with
  several people or environments, too much for one maintainer.
- **Revisit when.** Other people deploy, or there are several environments.

### 16. Cloudflare in front of the public hostnames

- **Decision.** The domain's name servers point to Cloudflare, which sits in
  front of every public hostname: the website, the API and the MCP server.
- **Why.** Public data that changes once a day suits a CDN well. The cluster
  receives less traffic and is not exposed directly.
- **To know.** All visitor traffic then passes through a company outside
  Switzerland, which weakens a "hosted in Switzerland" claim. The data itself
  is public.
- **Guardrail.** Nothing may depend on it. The website and the API send
  standard cache headers and work without it, and the Helm chart does not
  assume it.
- **Revisit when.** It gets in the way of API or MCP clients, or the project
  wants to state that no traffic leaves Switzerland. What Cloudflare caches,
  and how it connects to the cluster, are still open, see below.

### 17. AGPL-3.0 for the code

- **Decision.** The code is licensed under AGPL-3.0-only. The data keeps the
  terms of its source.
- **Why.** Anyone who runs a modified version as a network service must offer
  its source, so improvements stay open.
- **Rejected.** A permissive licence, which allows closed forks. A
  non-commercial licence, which is not open source. AGPL does not forbid
  commercial use.
- **Revisit when.** Nothing foreseeable.

### 18. A site's history is a list of periods between snapshots

- **Decision.** A snapshot is identified by its publication time. For each
  site the database keeps one row per period during which its published
  values stayed the same, bounded by the publication times of the snapshot
  that first showed them and of the first snapshot that no longer did. Whether
  a site is published today follows from its periods: it is when one of them
  has no end.
- **Why.** The file changes little from one day to the next: a few hundred
  sites out of 23,000. One row per site and per day would be 8 million rows a
  year that say nothing new. With periods, what was published at any moment is
  one comparison of dates, and what changed between two moments is one query.
- **Rejected.** One row per site and per snapshot. Numbering the snapshots
  and referring to the numbers: every question about a date would need the
  snapshot table. A "last seen" column on the site: one write per site and per
  day for something the periods already say.
- **Revisit when.** The detailed data of version 1.0 arrives: antennas and
  cells will need the same treatment, and may show its limits.
- **Accepted.** 25 November 2026.

### 19. The import only moves forward, and trusts nothing

- **Decision.** The import loads snapshots in the order they were published.
  What is loaded must be the beginning of the archive's index, snapshot for
  snapshot. The import therefore refuses a snapshot older than one that is
  loaded, an index that no longer lists a loaded snapshot, and a loaded
  snapshot whose checksum changed. It also refuses a file that is not the one
  the index describes, a snapshot dated more than a day in the future, and a
  file with less than half the sites published before it, unless a person
  allows that one file.
- **Why.** Each snapshot is recorded as differences from the one before, so
  inserting one in the middle would mean rewriting what follows. And the
  archive is written by an unattended job: a wrong or truncated file would
  otherwise end thousands of sites in a history that is meant to be kept.
- **Consequence.** To take in a snapshot found later, or to apply a parser fix
  to the past, load the archive into an empty database. That takes seconds
  per snapshot, and decision 1 is what makes it possible.
- **Rejected.** Rewriting periods in place when an older snapshot arrives:
  far more code, for a case the archive is built to avoid.
- **Revisit when.** Rebuilding takes too long to be the answer.
- **Accepted.** 25 November 2026.

### 20. Migrations are plain SQL files applied by the program

- **Decision.** The schema is a numbered list of SQL files, embedded in the
  program and applied by `swisscoverage migrate`, each in its own transaction.
  There is no way back: a change is a new file, and a merged file is never
  edited. The runner is about a hundred lines of our own.
- **Why.** The files are the single description of the schema: `sqlc` checks
  the queries against them. Rolling back is done by running the previous
  version of the program, which the schema must keep working with
  (decision 10), not by undoing a migration.
- **Rejected.** A migration library (golang-migrate, goose, tern): a
  dependency to follow for something this small, most of it for the way back
  we do not use.
- **Revisit when.** A migration must run outside a transaction, such as
  building an index without blocking writes on a large table.
- **Accepted.** 25 November 2026.

### 21. Huma describes the API

- **Decision.** The API is written with [Huma](https://huma.rocks), on the
  standard library's router. Its routes and the Go types of their parameters
  and responses are what Huma turns into the OpenAPI 3.1 contract
  (decision 8), and what it validates each request against.
- **Why.** One description gives the contract, the validation and the
  documentation of each field, so they cannot disagree. Only Huma's own
  packages are compiled into the program: its adapter for the standard
  library's router needs no other module, and the formats, routers and
  command-line helpers it can use are left out.
- **Rejected.** Writing the contract by hand and checking the handlers
  against it: decision 8 already rejected writing it first, and a generator of
  our own would be more code than the API. Generators that start from the
  contract (`ogen`, `oapi-codegen`): the same objection. Annotations in
  comments (`swaggo`): OpenAPI 2 only, and nothing checks them against the
  code.
- **To know.** Huma marks lists as nullable by default; the API turns that
  off, since every list it returns is a list, empty when there is nothing.
  Its pages of documentation, which load scripts from a CDN, and the
  `$schema` member it adds to responses are turned off too.
- **Revisit when.** Huma stops being maintained, or a version breaks the
  contract it generates.
- **Accepted.** 25 November 2026.

### 22. A station's identifier is its position and its label

- **Decision.** The API identifies a site by `east_north_label`: its LV95
  position as published, in metres, and its label, such as
  `2683328_1247916_Sunrise ZH601-1`. That is the identity of decision 11,
  written as one string.
- **Why.** OFCOM publishes no identifier. The database numbers its rows, but
  those numbers change when the archive is loaded again into an empty
  database, which is how a parser fix reaches the past (decision 19). The
  position and the label do not change, so a link to a station keeps working.
  The identifier also reads back into the site without a table to look it up
  in.
- **Rejected.** The database's numbers: they change on a rebuild. A hash of
  the label and the position: as stable, but opaque, and it needs a column
  to search by.
- **Limit.** The identifier is as long as the label, and holds what the label
  holds: a client percent-encodes it in a path. A site moved by a few metres
  gets a new identifier, as it becomes a new site (decision 11).
- **Revisit when.** OFCOM publishes identifiers, or decision 11 changes.
- **Accepted.** 25 November 2026.

### 23. Responses are cached by what they contain

- **Decision.** Every successful response carries an ETag computed from its
  body and may be reused for five minutes. A request that sends that ETag
  back gets 304 Not Modified. Errors are never cached. Every list is
  paginated, at most 1,000 items at a time, except the list of every station.
- **Why.** The data changes once a day, so nearly every request can be
  answered by a cache: the browser's, Cloudflare's or any other
  (decision 16). An ETag taken from the body changes exactly when the
  response does, after an import or after a new version of the program writes
  it differently, with no version number to remember to change. Five minutes
  is short enough for a new snapshot to show quickly and long enough to spare
  the database.
- **Cost.** The API builds a response before it can tell that it did not
  change: a 304 saves the transfer, not the work. Behind a cache that is
  rarely felt; without one, the list of every station costs about half a
  second of the database's time per request.
- **Rejected.** An ETag from the latest snapshot alone: cheaper, but wrong
  for a day after a deployment that changes a response.
- **Revisit when.** The work behind each request becomes the bottleneck. The
  API can then keep the latest response in memory.
- **Accepted.** 25 November 2026.

## Open

Not decided yet. Each has a recommendation and the step of the
[roadmap](roadmap.md) that settles it.

| Question                                                                            | Recommendation                                                                                                                                                | Settled in |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| A page per station, found by search engines, or only a map?                         | Pages, rendered on the server. Without them the site is a browser-only application and the `web` image serves static files.                                   | Step 4     |
| Which base map?                                                                     | The official swisstopo base map if its terms allow it, otherwise OpenFreeMap. The terms are not checked yet.                                                  | Step 4     |
| Which service turns an address into coordinates, and who calls it?                  | swisstopo's public search service, called from the browser, if its terms allow it. It is a new source: it goes into [data-sources.md](data-sources.md) first. | Step 4     |
| The API on `api.swisscoverage.ch` or under `swisscoverage.ch/api`?                  | The subdomain: the API's address then does not depend on the website.                                                                                         | Step 5     |
| Two small cluster nodes, or one?                                                    | Two, so that a cluster update does not interrupt the site. Prices are not checked yet.                                                                        | Step 5     |
| What does Cloudflare do: cache pages and API responses, or only CDN and protection? | Start with DNS, CDN for static files and protection. Decide on caching responses with real measurements.                                                      | Step 5     |
| Cloudflare as a plain proxy, or as a tunnel into the cluster?                       | The plain proxy. A tunnel saves a load balancer and a public address, but makes the site depend on Cloudflare, which would reopen decision 16's guardrail.    | Step 5     |
| Which source for cantons and municipalities?                                        | swisstopo's open boundaries dataset, loaded into PostGIS, if its terms allow it. It is a new source: it goes into [data-sources.md](data-sources.md) first.   | Step 8     |
