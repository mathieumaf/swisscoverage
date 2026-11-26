# Threat model

What swisscoverage protects, from whom, and where. It is the context for
security review of pull requests, and the place that explains the promises in
[SECURITY.md](../../SECURITY.md).

Revisit this page when exposure, identity, data access or privileged
operations change: in practice, in the pull request that adds the database, the
API, the MCP server or the deployment, or that changes how the archive is
read.

## System and scope

swisscoverage turns official data about mobile network sites in Switzerland
into a map, a public API and an MCP server.

**What exists today**

- `backend/internal/ofcom`: a pure Go package that parses OFCOM's public
  GeoJSON file into a typed model (`mobile_stations.go`). It has no network,
  file system, database or environment access, and uses the standard library
  only.
- `backend/cmd/swisscoverage`: the program. One command listens on the
  network, `serve`; the others are run by hand or by a job.
  - `parse` reads one file named by whoever runs it, or standard input, and
    prints what the parser makes of it. It writes no file and holds no
    credentials.
  - `migrate` changes the schema of the database named by `DATABASE_URL`.
  - `ingest` reads the archive over HTTPS and writes to that database.
  - `serve` answers HTTP requests with the public, read-only API, from that
    database. It listens on `127.0.0.1:8080` unless told otherwise.
  - `openapi` prints the API's contract. It reads nothing.
- `backend/internal/archive`: the client that reads the archive's index and
  files. It treats both as untrusted.
- `backend/internal/store`: the schema, its migrations and the queries, for
  PostgreSQL with PostGIS. Queries are written in SQL with parameters and
  turned into Go code by a generator; none is built from strings.
- `backend/internal/api`: the routes of the API, built with Huma, which
  validates each request against the types the contract is generated from.
  It reads through the store only.
- `web/`: the website, in TypeScript with Astro. Its pages are rendered by a
  server, which Bun runs, and which calls the public API at the origin named
  by `SWISSCOVERAGE_API_URL`, `http://127.0.0.1:8080` by default. It holds no
  credentials and has no access to the database. Locally it listens on
  `127.0.0.1:4321`. Today it serves the map, on the home page of each
  language. The map runs in the visitor's browser: it reads the stations
  from the same API origin, and the base map from OpenFreeMap
  (`tiles.openfreemap.org`).
- `deploy/compose.yaml`: a PostgreSQL with PostGIS for local work and tests,
  reachable from the machine it runs on only, with credentials written in the
  file. It holds nothing that cannot be loaded again from the archive.
- Continuous integration (`.github/workflows/ci.yml`): formatting, `go vet`,
  a check that the generated code is current, and tests for the backend
  against the database of `deploy/compose.yaml`; lint for the documentation
  and the TypeScript; for the website, a check that its types match the
  contract, type checks, tests and a build.
  It runs on every pull request and on `main`, with read-only permissions.
- The daily archive, in a separate repository,
  [`mathieumaf/swisscoverage-data`](https://github.com/mathieumaf/swisscoverage-data).
  A scheduled workflow there downloads OFCOM's file, commits it in a
  comparable form, lists it in an index of snapshots and attaches the original
  to a release
  (`scripts/archive.sh`, `.github/workflows/archive.yml`). Changes there do
  not go through this repository's pull requests or its CI.

**What is planned** ([architecture](../architecture.md)). Everything in this
list is an assumption until the code exists, and is marked _unverified_ below.

- The rest of the backend: `mcp`, the MCP server, as another command of the
  same program. `serve`, `ingest` and `migrate` exist, and are not deployed
  anywhere yet.
- The MCP server, and the website's station pages. Both are clients of the
  public API over HTTP and have no access to the database.
- Container images published to the GitHub container registry, deployed to a
  Kubernetes cluster with a Helm chart.
- Cloudflare in front of every public hostname, for DNS and as a CDN.

Nothing is deployed. There are no users, accounts, credentials or stored
personal data.

## Assets

| Asset                         | Why it matters                                                                 |
| ----------------------------- | ------------------------------------------------------------------------------ |
| Integrity of published data   | The project republishes data under OFCOM's name. Wrong data misleads its users |
| The archive of daily files    | The history cannot be downloaded again; a lost or altered day is lost for good |
| The repository and its CI     | Whoever controls them controls what is built and published                     |
| Release and deployment access | _Unverified._ Registry and cluster credentials, once they exist                |
| Availability of the API       | _Unverified._ Others will build on it                                          |

## Actors

- **Anonymous internet users.** Will reach the public API, the MCP server and
  the map. Untrusted.
- **Contributors.** Open pull requests from forks. Their code and fixtures are
  untrusted until reviewed and merged.
- **The maintainer.** Merges, releases and deploys. Trusted.
- **Automation.** Dependabot and workflows acting with the repository's token.
  Trusted only within the permissions each workflow declares.
- **Upstream publishers.** OFCOM's servers and the package registry. Their
  content is trusted for what it claims to be, not for being safe to execute
  or well formed.

## Entry points and untrusted input

| Entry point               | State          | Input                                                                                                                                                                                                                                                                        | Handling                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OFCOM file                | exists         | A GeoJSON document of about 28 MB                                                                                                                                                                                                                                            | `ParseMobileStations` validates the shape and the reference system, reads strings as data, and reports unknown labels as warnings                                                                                                                                                                                                                                                                                                                              |
| Samples in the repository | exists         | JSON samples added by contributors, under `testdata/`                                                                                                                                                                                                                        | Reviewed for provenance (`AGENTS.md`, Code Review Rules); parsed like any other file                                                                                                                                                                                                                                                                                                                                                                           |
| Pull requests from forks  | exists         | Code, workflows, lockfile                                                                                                                                                                                                                                                    | CI runs on `pull_request` with `contents: read` and no secrets; changes to workflows and dependencies are High risk                                                                                                                                                                                                                                                                                                                                            |
| Dependencies              | exists         | The Go toolchain, the `pgx` driver and Huma, and what they need, `sqlc` run by `go generate`, the database image, GitHub actions, Biome, and the website's packages (Astro, its Node adapter, React, MapLibre GL, `openapi-fetch` and their tools) from the package registry | CI installs the Go version that `backend/go.mod` names and refuses to download another (`GOTOOLCHAIN=local`); Go modules are verified against `go.sum`; `sqlc` is pinned to a version; the database image is pinned to a digest; locked installs of the JavaScript packages, at the root and in `web/` (`bun install --frozen-lockfile`), each direct one pinned to an exact version; actions pinned to a commit; Dependabot proposes updates as pull requests |
| Daily archive job         | exists         | OFCOM's catalogue and file, fetched unattended                                                                                                                                                                                                                               | In `swisscoverage-data`: checks the address, dates and checksum format before use, verifies the file's SHA-256 against the catalogue, and passes values to steps through the environment                                                                                                                                                                                                                                                                       |
| Archive index and files   | exists         | The index of snapshots and the gzipped originals, fetched by `ingest` over HTTPS                                                                                                                                                                                             | `backend/internal/archive` checks every value, bounds every download and verifies each file's SHA-256; `ingest` refuses what would rewrite or empty the history. See trust boundary 8                                                                                                                                                                                                                                                                          |
| Database                  | exists locally | The address in `DATABASE_URL`; rows written by `ingest`                                                                                                                                                                                                                      | Parameterised SQL only, generated from `queries.sql`; the schema constrains every value the parser can return; a snapshot is written in one transaction                                                                                                                                                                                                                                                                                                        |
| Public API                | exists locally | The path, the query parameters and the headers of anonymous requests                                                                                                                                                                                                         | Every parameter is validated against the contract, unknown ones refused; parameterised SQL only; read-only transactions on connections that refuse writes; every list paginated at 1,000 items, except every current station at once, which the dataset bounds; errors without internals. See trust boundary 4                                                                                                                                                 |
| MCP server                | _unverified_   | Tool arguments chosen by an AI model                                                                                                                                                                                                                                         | Must treat arguments as untrusted, reach data only through the public API, and return API data as data, never as instructions to follow                                                                                                                                                                                                                                                                                                                        |
| Website server            | exists locally | Request paths, the `Accept-Language` header, and responses from the API                                                                                                                                                                                                      | A language in the path that is not one of the four answers 404; `Accept-Language` only chooses among the four; calls go to the origin of `SWISSCOVERAGE_API_URL` only, checked on first use to be http or https without path or credentials, with paths from the contract; each call gives up after 5 seconds; a failed call is logged and the page says only that the data cannot be loaded; Astro escapes every value it renders. See trust boundary 6       |
| Website in the browser    | exists locally | Station labels and other strings from the API; the style, tiles, fonts and sprites of the base map                                                                                                                                                                           | React renders every string from the API as text; the map's own layers draw positions and colours only. The browser calls the API at the origin the server gives it, with paths from the contract and no redirects, and gives up after 60 seconds. The base map comes from the one style address in the code; MapLibre draws what it receives and runs none of it                                                                                               |

## Trust boundaries

1. **Source file → parser.** The file crosses from the network into the
   project's model. Enforced today in `backend/internal/ofcom`: nothing from
   the file is evaluated, and a file in another reference system is rejected.
   The parser holds the whole file in memory, which is fine for a file someone
   chose to read by hand. The import bounds the size of what it downloads
   before it parses it (boundary 8).
2. **Fork → repository.** A pull request crosses when it is merged. Enforced by
   the maintainer's review and by the `check` job. Branch protection on `main`
   is what makes both mandatory. It is a repository setting, not a file: it
   does not show in a diff, so confirm it is in place when it matters.
3. **Repository → published artefact.** _Unverified._ A commit will cross when
   CI builds and publishes an image. The release workflow must run only on the
   protected branch, with the narrowest token that works.
4. **Internet → API.** Requests cross into SQL queries. Enforced today in
   `backend/internal/api` and the `serve` command, and not yet exposed to the
   internet, since nothing is deployed:
   - every parameter is validated against the types the contract comes from:
     ranges, vocabularies, lengths, and the pairs that go together, such as a
     point's latitude and longitude. NaN and infinities are refused. An
     unknown parameter is refused too, and so are one given twice, one left
     empty and a query string that does not decode;
   - values reach SQL as parameters of the queries of `queries.sql`, never in
     the text of a query;
   - every list is paginated, at most 1,000 items, and a search around a point
     reaches 50 km at most. The list of every current station is the one
     unbounded response, by design: the dataset bounds it, at about 6 MB;
   - the database stops a statement after 10 seconds, and the API a request
     after 20. Headers have 10 seconds to arrive, and 16 KB at most;
   - an error that the client cannot act on is logged and answered without
     its details;
   - responses are held in memory to compute their ETag, which their bounds
     make safe.

   What this does not cover: there is no rate limit. Each request that is not
   answered by a cache is built in full, a 304 included, and the list of every
   station costs about half a second of the database's time. Without a cache
   in front, a client that asks for it in a loop can make the API slow for
   everyone. Rate limits and the cache in front are for roadmap step 5, with
   the deployment.
5. **API → database.** `ingest`, `migrate` and `serve` connect with
   whatever role `DATABASE_URL` names; no deployment exists yet to give each
   its own. The API's connections already refuse to write
   (`default_transaction_read_only`), and it reads in read-only transactions,
   but a role that can only read is what makes that hold against a bug in the
   program: it comes with the deployment. It is the only service exposed to the internet that connects
   to the database. The import writes data and the migrations change the
   schema, each with its own role, and neither is reachable from the internet.
6. **MCP server and website → API.** Both call the public API like any third
   party. Neither may hold database credentials, so a flaw in either exposes
   nothing the API does not already publish. Enforced today for the website,
   which is a separate program in `web/`, with no database driver among its
   dependencies: it reaches the API at one origin, set by whoever runs it, and
   nothing a visitor sends changes that origin or adds a path outside the
   contract. The browser reaches the same origin. What this does not cover
   yet: the website sends no security headers of its own, such as a Content
   Security Policy. They come with the deployment, which fixes the origins a
   policy must allow: the API's and the base map's. _Unverified_
   for the MCP server: it ships in the same image as the API, the import and
   the migrations, so this holds only if each deployment gets its own
   credentials and network access, and if the MCP package cannot import the
   database code.
7. **Internet → Cloudflare → cluster.** _Unverified._ Cloudflare will
   terminate visitors' TLS and forward requests to the cluster. It then sees
   all visitor traffic, and a response it caches is served without reaching
   the cluster. Nothing private may ever be sent with a header that allows
   caching, and the cluster should accept traffic only from where it is meant
   to come. The API sends `Cache-Control: public` on successful responses,
   which hold public data only, and `no-store` on errors.
8. **Archive → import.** The import reads the archive's index and files.
   That repository's workflow pushes to its `main` branch with a write token
   and without review, so the import treats the archive as untrusted input.
   Enforced today in `backend/internal/archive` and in the `ingest` command:
   - every value of the index is checked against a strict pattern, and the
     two that become part of an address cannot contain a path or a host;
   - the index, the compressed file and the decompressed file are each read up
     to a fixed size;
   - a file is parsed only when its SHA-256 is the one the index gives, and
     loaded only when its number of sites is too;
   - what is loaded must be the beginning of the index: a loaded snapshot
     whose checksum changes there, one that the index no longer lists, and a
     snapshot older than what is loaded each stop the import;
   - a file with less than half the sites published before it is not loaded
     until a person allows it, by naming that snapshot's publication time.
     Sites are counted once each, so repeating a site does not hide a loss;
   - a snapshot dated more than a day in the future is not loaded.

   What this does not cover: whoever can push to the archive can publish an
   index and a file that agree with each other. The checksum proves the file
   is the one the index describes, not that OFCOM published it. That
   comparison is made once, by the archive's job, against OFCOM's catalogue.

## Sensitive data flows and privileged actions

- **No personal data today.** The dataset describes infrastructure, not people.
  Operators are legal entities, and site locations are published by OFCOM.
- **Data from Freedom of Information requests.** _Unverified._ The detailed
  antenna database, once obtained, may be published within copyright and data
  protection law. Check each new column for personal data before publishing it.
- **The address of the database is a credential.** `DATABASE_URL` holds a
  password. The program reads it from the environment only, never from an
  argument, and never prints it itself. The errors of the driver are shown,
  and the driver hides the password of an address it can read: tests check
  it for a failed connection and for an address with a wrong port. A password
  that is not percent-encoded can make the address unreadable in a way that
  shows part of it, on the terminal of whoever typed it.
- **Privileged actions.** Today: merging to `main`, changing workflows or
  repository settings, the archive workflow's pushes and release uploads in
  `swisscoverage-data` (`contents: write`), and running `migrate` or `ingest`,
  which change whatever database `DATABASE_URL` names. Planned and
  _unverified_: publishing images, deploying to the cluster, running the
  migrations and the import there.

## Accepted risks and exclusions

- **Antenna locations are public.** The project does not treat republishing
  OFCOM's open data as a risk it must mitigate.
- **Upstream errors are republished.** The project does not correct OFCOM's
  data. It documents known quirks in [data-sources.md](../data-sources.md).
- **A single maintainer.** A compromised maintainer account defeats every
  control here. Mitigated outside the repository, by account security.
- **A third party behind the map.** Each visitor's browser asks OpenFreeMap,
  through Cloudflare, for the tiles of the area they look at, which reveals
  their address and the area to that service. Accepted because the map needs
  a base map and the project has no tile server; OpenFreeMap keeps no IP
  address in its access logs ([data-sources.md](../data-sources.md)). A
  self-hosted OpenFreeMap removes the third party, if that becomes needed.
- **A third party in front of the site.** _Unverified._ Putting Cloudflare in
  front means trusting it with visitor traffic and with the availability of
  the site. Accepted because the data is public; to be reconsidered if the
  project ever handles personal data.
