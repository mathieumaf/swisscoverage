# swisscoverage

An open map, API and MCP server for mobile network antennas in Switzerland,
built on OFCOM's open data. The project is at its beginning. Today the code is
the backend, in Go under `backend/`: the `swisscoverage` program with five
commands, `serve`, `openapi`, `parse`, `migrate` and `ingest`; the parser for
OFCOM's file; the database, PostgreSQL with PostGIS, with the import that
loads the archive into it; and the public, read-only API, with its contract
in `openapi.json`. The website, in TypeScript with Astro under `web/`, is
started: its pages are rendered on a server that calls the API, and its home
page is the map of every station. The MCP server is planned.

- `docs/architecture.md`: read before adding a module, a command or a
  dependency. It says what exists, what is planned, and where a change belongs.
- `docs/decisions.md`: read before proposing another language, framework or
  service. It says why each choice was made and what would reopen it.
- `docs/roadmap.md`: the order of work, and what "done" means for each step.
- `docs/data-sources.md`: read before touching a parser or adding data. It
  describes each source file and the rule on which data may be used.

Everything in the repository and on GitHub is written in English.

## Proof

Run with the Go version of `backend/go.mod`, Bun 1.4.5 and Docker. The change
is good when each command exits 0 and prints what is expected.

Start the local database first. It is the one of `deploy/compose.yaml`, and
it holds nothing that cannot be loaded again:

```sh
docker compose -f deploy/compose.yaml up -d --wait
```

| Command                 | Run in     | Proves                                                                        | Expected                     |
| ----------------------- | ---------- | ----------------------------------------------------------------------------- | ---------------------------- |
| `CI=true go test ./...` | `backend/` | the tests, those on the database included, and that `openapi.json` is current | prints `ok` for each package |
| `go vet ./...`          | `backend/` | no suspicious construct                                                       | prints nothing               |
| `gofmt -l .`            | `backend/` | formatting of the Go code                                                     | prints nothing               |
| `go mod tidy -diff`     | `backend/` | `go.mod` and `go.sum` match the imports                                       | prints nothing               |
| `go generate ./...`     | `backend/` | the generated query code is current                                           | `git status` shows no change |
| `bun run check`         | the root   | lint and formatting of Markdown, JSON, YAML and TypeScript                    | prints `No fixes applied.`   |
| `bun run generate`      | `web/`     | the website's types match `openapi.json`                                      | `git status` shows no change |
| `bun run check`         | `web/`     | the type checks of the website, `.astro` files included                       | prints `0 errors`            |
| `bun test`              | `web/`     | the website's tests                                                           | prints `0 fail`              |
| `bun run build`         | `web/`     | the website builds for production                                             | prints `Complete!`           |

Run `bun install --frozen-lockfile` at the root and in `web/` first.

Without `CI=true`, the tests that need the database are skipped when it does
not answer, and `go test` still prints `ok`: that proves nothing about the
database. With it, they fail instead. Each test gets a database of its own,
created beside the local one and dropped when the test ends.

`gofmt -l .` exits 0 even when it lists files: read its output. `gofmt -w .`
fixes the Go formatting and `bun run format` fixes the rest.

To see a parser change against real data, parse today's public file:

```sh
(set -o pipefail; curl -sSf https://data.geo.admin.ch/ch.bakom.standorte-mobilfunkanlagen/standorte-mobilfunkanlagen/standorte-mobilfunkanlagen_2056.json | go run -C backend ./cmd/swisscoverage parse -)
```

It prints about `23000 stations, 0 warnings`, then the counts per operator,
kind and technology, and exits 0. It exits with an error when the download
fails or the file is not the dataset. Warnings do not change the exit status:
read the first line. A warning means the file contains a label the parser does
not know: add it, with a sample taken from that file.

To show that a parser change alters nothing else, save that file once, run
`swisscoverage parse -json <file>` before and after the change, and compare the
two outputs.

`go test ./internal/ofcom -update`, run in `backend/`, rewrites the golden
file from what the parser returns. Its diff is then part of the change: read
it.

`openapi.json`, at the root, is the API's contract as
`swisscoverage openapi` prints it, and a test fails when the two differ. After
a change to the API, `go run ./cmd/swisscoverage openapi > ../openapi.json`,
run in `backend/`, writes it again. Its diff is the change to the public
contract: read it. It may add, never remove or change what exists.

To see a change to the schema or to the import against real data, load the
archive into the local database:

```sh
export DATABASE_URL=postgres://swisscoverage:swisscoverage@127.0.0.1:54320/swisscoverage
go run -C backend ./cmd/swisscoverage migrate
go run -C backend ./cmd/swisscoverage ingest
```

`ingest` prints one line per snapshot it loads, with the sites that appeared,
disappeared and changed, and exits 0. Run it again: it prints
`Nothing to load` and changes nothing. It downloads each snapshot once, from
the project's own archive. To start from nothing, throw the database away
with `docker compose -f deploy/compose.yaml down -v` and start it again.

To see a change to the API against real data, load the archive as above,
then serve it with the same `DATABASE_URL`:

```sh
go run -C backend ./cmd/swisscoverage serve
```

It prints `Serving the API on http://127.0.0.1:8080/v1/` and answers until it
is interrupted. In another terminal, each endpoint answers 200:

```sh
curl -sS -o /dev/null -w '%{http_code} %{size_download}\n' http://127.0.0.1:8080/v1/stations
curl -sS 'http://127.0.0.1:8080/v1/stations/search?lat=46.948&lon=7.4474&radius=300&limit=2'
curl -sS 'http://127.0.0.1:8080/v1/stations/2683328_1247916_Sunrise%20ZE416-1'
curl -sS http://127.0.0.1:8080/v1/stations/counts
curl -sS http://127.0.0.1:8080/v1/snapshots
```

The first prints about `200 5600000`: every station, as GeoJSON. A search
names a station's `id`, which reads it back once percent-encoded. A request
with a parameter that is wrong or unknown answers 422. `serve` only reads:
its connections refuse every write.

To see a change to the website, serve the API as above, then, in `web/`:

```sh
bun run dev
```

It serves the site on `http://127.0.0.1:4321/`, against the API at
`http://127.0.0.1:8080`; `SWISSCOVERAGE_API_URL` names another origin, such
as `http://127.0.0.1:8081`. Astro may start it in the background and print
how to stop it, `bunx --bun astro dev stop`. In another terminal:

```sh
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' -H 'Accept-Language: de-CH' http://127.0.0.1:4321/
curl -sS http://127.0.0.1:4321/fr/ | grep -o 'id="snapshot"[^<]*'
```

The first prints `302 http://127.0.0.1:4321/de/`: the root sends each visitor
to their language. The second prints the number of sites and the date of the
latest snapshot. Open `http://127.0.0.1:4321/fr/` in a browser to see the
map: the browser itself loads every station from the API, at the origin of
`SWISSCOVERAGE_API_URL`, and the base map from OpenFreeMap. A click on a
station opens its sheet. With the API stopped, the page still answers 200 and says the
data cannot be loaded. `bun run build` then `bun run preview` serves the
production build the same way.

Everything above is safe: the downloads are public, read-only files, what is
written goes to the working tree or to the local database, and `serve`
listens on this machine only, like the website, which calls the API named by
`SWISSCOVERAGE_API_URL` and nothing else. The repository holds no credentials
beyond the ones of that local database, and there is no deployment today, so
no command touches production.

Off limits:

- Fetching data from any map, website or app other than the sources listed in
  `docs/data-sources.md`, including to build a fixture or to compare results.
- Downloading the OFCOM file in a loop or in tests. Tests read `testdata/`,
  and the tests of the import read an archive they serve themselves.
- Setting `DATABASE_URL` or `SWISSCOVERAGE_TEST_DATABASE_URL` to anything but
  the local database of `deploy/compose.yaml`. `migrate` changes a schema,
  `ingest` writes, `serve` publishes what it reads, and the tests create and
  drop databases on the server they are given.
- Running `serve` on an address other machines can reach, such as
  `-listen :8080`, outside a deployment, and the same for the website:
  `astro dev --host`, or `HOST=0.0.0.0` for the production build.
- Setting `SWISSCOVERAGE_API_URL` to an API you do not run yourself.

When a deployment is added, the pull request that adds it must extend this
section: how to run it against throwaway data, and which commands touch
production and are off limits.

## Code Review Rules

Flag a change when:

- **It loses what the source said.** A parser drops a station, or fails,
  because of one label it does not recognise, so a new upstream value blocks
  the whole daily import. The expected behaviour is to keep the station, keep
  what the parser does recognise, set the rest to `unknown`, `nil` or an empty
  list, and add a warning.
- **It invents meaning.** A parser guesses a value the file does not state:
  an operator for a bare station code, a technology, a default power class. Or
  it treats the station label as unique. One label appears at several
  positions; the pair of label and position is the smallest safe identity.
- **It misplaces sites.** Coordinates are read without checking that the file
  is in EPSG:2056, east and north are swapped, or LV95 is converted with
  approximate formulas instead of being left to PostGIS.
- **It brings in data we may not use.** A fixture or data file that does not
  come from a source documented in `docs/data-sources.md`, anything scraped
  from another map or app, or a change that drops the attribution to OFCOM,
  or the base map's to OpenFreeMap, OpenMapTiles and OpenStreetMap.
- **It breaks the parser's boundary.** The parser (`backend/internal/ofcom`)
  reads the network, the file system, the environment, a database or the
  clock. It must stay pure so its tests are deterministic. Reading files is
  the job of the command that calls it, and of its tests.
- **It opens a second path to the data.** The website or the MCP server
  imports backend code or connects to the database instead of calling the
  public API. Both are clients of the API, like any third party.
- **It breaks the schema's promises.** A migration that is already merged is
  edited instead of followed by a new one. Or a new migration removes,
  renames or tightens something the previous version of the program relies
  on: two versions run side by side during an update and after a rollback.
- **It rewrites history.** The import changes a period that is already
  recorded for anything but ending it, loads a snapshot out of order, or
  writes a snapshot in more than one transaction.
- **It trusts the archive.** A value from the archive's index is used before
  it is checked, a file is parsed before its checksum is verified, or a
  download has no bound. The archive is written by an unattended job.
- **It builds SQL from strings.** Queries live in
  `backend/internal/store/queries.sql`, with parameters.
- **It breaks the API's promises.** A change to `openapi.json` removes,
  renames or changes the meaning of something it already describes: clients
  built on the previous contract keep running. Or a response that carries
  data loses OFCOM's attribution, a list loses its bound on size, a parameter
  goes unvalidated, an error shows the server's internals, or an error is
  sent with a header that lets it be cached.
- **It lets the API write.** The API reads through
  `store.OpenReadOnly` and read-only transactions only.
- **It trusts what the website receives.** A page renders a string from the
  API or from the request as HTML (`set:html`, `dangerouslySetInnerHTML`),
  builds the address of an API call from anything but `SWISSCOVERAGE_API_URL`
  and the paths of the contract, or shows a visitor the details of an error.

Skip third-party texts: `LICENSE` and `CODE_OF_CONDUCT.md`. For the samples
under `**/testdata/` and `**/fixtures/`, check where they come from, not their
formatting. A `*.golden.json` file records what the parser returns: review a
change to it as a change to the parser's behaviour. Review `bun.lock` and
`backend/go.sum`: they pick the dependency versions that run. `openapi.json`
is generated: review it as the change to the public contract that it shows,
and check that it changed only with `backend/internal/api` or with the
parser's vocabularies, which it lists. `web/src/api/schema.ts` is generated
from it: check that it changed only with `openapi.json`. The code under
`backend/internal/store/query/` is generated: review `queries.sql` and the
migrations it comes from, and check that it changed only with them.

Cite file:line for every finding, and post at most 3 nits.

## Risk

When paths overlap, the higher tier wins.

**High**

- `.github/workflows/**`, `.github/dependabot.yml`: what runs with the
  repository's token, and what may merge.
- `backend/go.mod`, `backend/go.sum`, `package.json`, `bun.lock`: the Go
  version and the dependencies that run.
- `AGENTS.md`, `CLAUDE.md`, `SECURITY.md`, `docs/security/**`: they steer
  review and shipping.
- `docs/data-sources.md`: the list of sources the project may use. Adding one
  there is what allows its data in.
- `LICENSE`: the terms of the whole project.
- `backend/internal/store/**`: the schema, its migrations, and every query
  that writes to the database.
- `openapi.json` and `backend/internal/api/**`: the public API's contract, and
  what answers requests from the internet.
- `deploy/**` and any `Dockerfile`: what runs, locally today, and what is
  released and how.
- `web/package.json`, `web/bun.lock`: the website's dependencies, and what
  runs them.

**Low**

- `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and `docs/**` other
  than the High and Medium paths.
- `.github/ISSUE_TEMPLATE/**`, `.github/PULL_REQUEST_TEMPLATE.md`.
- `**/*_test.go` and `**/*.test.ts`.

**Medium**

- Everything not listed in High or Low: for example the other Go files under
  `backend/`, the website's code under `web/src/`, `web/astro.config.ts`, and
  `biome.json`.
- `docs/architecture.md` and `docs/decisions.md`: agents and contributors act
  on them, so a wrong statement there misdirects real work.
- `**/testdata/**` and `**/fixtures/**`: a sample is data entering the
  repository, so its origin gets a real review, and a golden file is the
  parser's behaviour written down.
