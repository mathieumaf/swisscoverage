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

- `packages/core`: a pure TypeScript library that parses OFCOM's public GeoJSON
  file into a typed model (`packages/core/src/ofcom/mobile-stations.ts`). It
  has no network, file system, database or environment access.
- Continuous integration (`.github/workflows/ci.yml`): lint, types and tests on
  every pull request and on `main`, with read-only permissions.
- The daily archive, in a separate repository,
  [`mathieumaf/swisscoverage-data`](https://github.com/mathieumaf/swisscoverage-data).
  A scheduled workflow there downloads OFCOM's file once a day, commits it in a
  comparable form and attaches the original to a release
  (`scripts/archive.sh`, `.github/workflows/archive.yml`). Changes there do
  not go through this repository's pull requests or its CI.

**What is planned** ([architecture](../architecture.md)). Everything in this
list is an assumption until the code exists, and is marked _unverified_ below.

- A backend in Go, one program with four commands in production: `serve`, the
  public, read-only HTTP API; `mcp`, the MCP server; `ingest`, the import that
  loads snapshots into PostgreSQL with PostGIS; and `migrate`, which changes
  the schema.
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

| Entry point                | State        | Input                                                           | Handling                                                                                                                                                                                 |
| -------------------------- | ------------ | --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OFCOM file                 | exists       | A GeoJSON document of about 28 MB                               | `parseMobileStations` validates the shape and the reference system, reads strings as data, and reports unknown labels as warnings                                                        |
| Fixtures in the repository | exists       | JSON samples added by contributors                              | Reviewed for provenance (`AGENTS.md`, Code Review Rules); parsed like any other file                                                                                                     |
| Pull requests from forks   | exists       | Code, workflows, lockfile                                       | CI runs on `pull_request` with `contents: read` and no secrets; changes to workflows and dependencies are High risk                                                                      |
| Dependencies               | exists       | Packages from the registry, GitHub actions                      | Locked install (`bun install --frozen-lockfile`); actions pinned to a commit; Dependabot proposes updates as pull requests                                                               |
| Daily archive job          | exists       | OFCOM's catalogue and file, fetched unattended                  | In `swisscoverage-data`: checks the address, dates and checksum format before use, verifies the file's SHA-256 against the catalogue, and passes values to steps through the environment |
| Public API                 | _unverified_ | Query parameters                                                | Must validate every parameter, use parameterised SQL, bound result sizes, and stay read-only                                                                                             |
| MCP server                 | _unverified_ | Tool arguments chosen by an AI model                            | Must treat arguments as untrusted, reach data only through the public API, and return API data as data, never as instructions to follow                                                  |
| Website server             | _unverified_ | Request paths, query parameters and the language in the address | Must validate them, and build its calls to the API only from a fixed base address, never from what a visitor sends                                                                       |
| Website in the browser     | _unverified_ | Station labels and other strings from the API                   | Must render them as text, never as HTML                                                                                                                                                  |

## Trust boundaries

1. **Source file → parser.** The file crosses from the network into the
   project's model. Enforced today in `packages/core`: nothing from the file is
   evaluated, and a file in another reference system is rejected.
2. **Fork → repository.** A pull request crosses when it is merged. Enforced by
   the maintainer's review and by the `check` job. Branch protection on `main`
   is what makes both mandatory. It is a repository setting, not a file: it
   does not show in a diff, so confirm it is in place when it matters.
3. **Repository → published artefact.** _Unverified._ A commit will cross when
   CI builds and publishes an image. The release workflow must run only on the
   protected branch, with the narrowest token that works.
4. **Internet → API.** _Unverified._ Requests will cross into SQL queries.
5. **API → database.** _Unverified._ The API should connect with a role that
   can only read. It is the only service exposed to the internet that connects
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
8. **Archive → import.** _Unverified._ The import will read snapshots from the
   archive repository. That repository's workflow pushes to its `main` branch
   with a write token and without review, so the import must treat the archive
   as untrusted input, like the source file itself.

## Sensitive data flows and privileged actions

- **No personal data today.** The dataset describes infrastructure, not people.
  Operators are legal entities, and site locations are published by OFCOM.
- **Data from Freedom of Information requests.** _Unverified._ The detailed
  antenna database, once obtained, may be published within copyright and data
  protection law. Check each new column for personal data before publishing it.
- **Privileged actions.** Today: merging to `main`, changing workflows or
  repository settings, and the archive workflow's pushes and release uploads in
  `swisscoverage-data` (`contents: write`). Planned and _unverified_:
  publishing images, deploying to the cluster, running database migrations.

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
