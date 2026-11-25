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
- `backend/cmd/swisscoverage`: the program, with three commands so far, all
  run by hand or by a job, none listening on anything.
  - `parse` reads one file named by whoever runs it, or standard input, and
    prints what the parser makes of it. It writes no file and holds no
    credentials.
  - `migrate` changes the schema of the database named by `DATABASE_URL`.
  - `ingest` reads the archive over HTTPS and writes to that database.
- `backend/internal/archive`: the client that reads the archive's index and
  files. It treats both as untrusted.
- `backend/internal/store`: the schema, its migrations and the queries, for
  PostgreSQL with PostGIS. Queries are written in SQL with parameters and
  turned into Go code by a generator; none is built from strings.
- `deploy/compose.yaml`: a PostgreSQL with PostGIS for local work and tests,
  reachable from the machine it runs on only, with credentials written in the
  file. It holds nothing that cannot be loaded again from the archive.
- Continuous integration (`.github/workflows/ci.yml`): formatting, `go vet`,
  a check that the generated code is current, and tests for the backend
  against the database of `deploy/compose.yaml`; lint for the documentation.
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

- The rest of the backend, as two more commands of the same program that run
  in production: `serve`, the public, read-only HTTP API, and `mcp`, the MCP
  server. `ingest` and `migrate` exist, and are not deployed anywhere yet.
- The MCP server and a website. Both are clients of the public API over HTTP
  and have no access to the database. The website is a server exposed to the
  internet, not only a page in a browser.
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

| Entry point               | State          | Input                                                                                                                                                      | Handling                                                                                                                                                                                                                                                                                                                                                             |
| ------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OFCOM file                | exists         | A GeoJSON document of about 28 MB                                                                                                                          | `ParseMobileStations` validates the shape and the reference system, reads strings as data, and reports unknown labels as warnings                                                                                                                                                                                                                                    |
| Samples in the repository | exists         | JSON samples added by contributors, under `testdata/`                                                                                                      | Reviewed for provenance (`AGENTS.md`, Code Review Rules); parsed like any other file                                                                                                                                                                                                                                                                                 |
| Pull requests from forks  | exists         | Code, workflows, lockfile                                                                                                                                  | CI runs on `pull_request` with `contents: read` and no secrets; changes to workflows and dependencies are High risk                                                                                                                                                                                                                                                  |
| Dependencies              | exists         | The Go toolchain, the `pgx` driver and what it needs, `sqlc` run by `go generate`, the database image, GitHub actions, and Biome from the package registry | CI installs the Go version that `backend/go.mod` names and refuses to download another (`GOTOOLCHAIN=local`); Go modules are verified against `go.sum`; `sqlc` is pinned to a version; the database image is pinned to a digest; locked install of Biome (`bun install --frozen-lockfile`); actions pinned to a commit; Dependabot proposes updates as pull requests |
| Daily archive job         | exists         | OFCOM's catalogue and file, fetched unattended                                                                                                             | In `swisscoverage-data`: checks the address, dates and checksum format before use, verifies the file's SHA-256 against the catalogue, and passes values to steps through the environment                                                                                                                                                                             |
| Archive index and files   | exists         | The index of snapshots and the gzipped originals, fetched by `ingest` over HTTPS                                                                           | `backend/internal/archive` checks every value, bounds every download and verifies each file's SHA-256; `ingest` refuses what would rewrite or empty the history. See trust boundary 8                                                                                                                                                                                |
| Database                  | exists locally | The address in `DATABASE_URL`; rows written by `ingest`                                                                                                    | Parameterised SQL only, generated from `queries.sql`; the schema constrains every value the parser can return; a snapshot is written in one transaction                                                                                                                                                                                                              |
| Public API                | _unverified_   | Query parameters                                                                                                                                           | Must validate every parameter, use parameterised SQL, stay read-only, and bound the size of every response. One response is exempt by design: every current station at once, which the dataset itself bounds                                                                                                                                                         |
| MCP server                | _unverified_   | Tool arguments chosen by an AI model                                                                                                                       | Must treat arguments as untrusted, reach data only through the public API, and return API data as data, never as instructions to follow                                                                                                                                                                                                                              |
| Website server            | _unverified_   | Request paths, query parameters and the language in the address                                                                                            | Exists only if pages are rendered on the server, which is still open. It must then validate what it receives, and build its calls to the API only from a fixed base address, never from what a visitor sends                                                                                                                                                         |
| Website in the browser    | _unverified_   | Station labels and other strings from the API                                                                                                              | Must render them as text, never as HTML                                                                                                                                                                                                                                                                                                                              |

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
4. **Internet → API.** _Unverified._ Requests will cross into SQL queries.
5. **API → database.** _Unverified._ `ingest` and `migrate` connect with
   whatever role `DATABASE_URL` names; no deployment exists yet to give each
   its own. The API should connect with a role that can only read. It is the only service exposed to the internet that connects
   to the database. The import writes data and the migrations change the
   schema, each with its own role, and neither is reachable from the internet.
6. **MCP server and website → API.** _Unverified._ Both call the public API
   like any third party. Neither may hold database credentials, so a flaw in
   either exposes nothing the API does not already publish. The MCP server
   ships in the same image as the API, the import and the migrations, so this
   holds only if each deployment gets its own credentials and network access,
   and if the MCP package cannot import the database code.
7. **Internet → Cloudflare → cluster.** _Unverified._ Cloudflare will
   terminate visitors' TLS and forward requests to the cluster. It then sees
   all visitor traffic, and a response it caches is served without reaching
   the cluster. Nothing private may ever be sent with a header that allows
   caching, and the cluster should accept traffic only from where it is meant
   to come.
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
   - a loaded snapshot whose checksum changes in the index, and a snapshot
     older than what is loaded, stop the import;
   - a file with less than half the sites of the one before is not loaded
     until a person allows it.

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
  argument, and does not print it: a test checks that a failed connection
  does not show it.
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
- **A third party in front of the site.** _Unverified._ Putting Cloudflare in
  front means trusting it with visitor traffic and with the availability of
  the site. Accepted because the data is public; to be reconsidered if the
  project ever handles personal data.
