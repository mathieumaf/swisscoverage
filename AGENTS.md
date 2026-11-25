# swisscoverage

An open map, API and MCP server for mobile network antennas in Switzerland,
built on OFCOM's open data. The project is at its beginning. Today the code is
the start of the backend, in Go under `backend/`: the `swisscoverage` program
with one command, `parse`, and the parser for OFCOM's file. The database, the
API, the MCP server and a website in TypeScript are planned.

- `docs/architecture.md`: read before adding a module, a command or a
  dependency. It says what exists, what is planned, and where a change belongs.
- `docs/decisions.md`: read before proposing another language, framework or
  service. It says why each choice was made and what would reopen it.
- `docs/roadmap.md`: the order of work, and what "done" means for each step.
- `docs/data-sources.md`: read before touching a parser or adding data. It
  describes each source file and the rule on which data may be used.

Everything in the repository and on GitHub is written in English.

## Proof

Run with the Go version of `backend/go.mod` and with Bun 1.4.5. The change is
good when each command exits 0 and prints what is expected.

| Command             | Run in     | Proves                                      | Expected                     |
| ------------------- | ---------- | ------------------------------------------- | ---------------------------- |
| `go test ./...`     | `backend/` | the tests                                   | prints `ok` for each package |
| `go vet ./...`      | `backend/` | no suspicious construct                     | prints nothing               |
| `gofmt -l .`        | `backend/` | formatting of the Go code                   | prints nothing               |
| `go mod tidy -diff` | `backend/` | `go.mod` and `go.sum` match the imports     | prints nothing               |
| `bun run check`     | the root   | lint and formatting of Markdown, JSON, YAML | prints `No fixes applied.`   |

`gofmt -l .` exits 0 even when it lists files: read its output. `gofmt -w .`
fixes the Go formatting and `bun run format` fixes the rest.

There is no service to run yet. To see a parser change against real data,
parse today's public file:

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

Everything above is safe: the download is a public, read-only file, fetched
once, and nothing is written outside the working tree. The repository holds no
credentials, no database and no deployment today.

Off limits:

- Fetching data from any map, website or app other than the sources listed in
  `docs/data-sources.md`, including to build a fixture or to compare results.
- Downloading the OFCOM file in a loop or in tests. Tests read `testdata/`.

When a database, an API or a deployment is added, the pull request that adds it
must extend this section: how to run it against throwaway data, and which
commands touch production and are off limits.

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
  from another map or app, or a change that drops the attribution to OFCOM.
- **It breaks the parser's boundary.** The parser (`backend/internal/ofcom`)
  reads the network, the file system, the environment, a database or the
  clock. It must stay pure so its tests are deterministic. Reading files is
  the job of the command that calls it, and of its tests.
- **It opens a second path to the data.** The website or the MCP server
  imports backend code or connects to the database instead of calling the
  public API. Both are clients of the API, like any third party.

Skip third-party texts: `LICENSE` and `CODE_OF_CONDUCT.md`. For the samples
under `**/testdata/` and `**/fixtures/`, check where they come from, not their
formatting. A `*.golden.json` file records what the parser returns: review a
change to it as a change to the parser's behaviour. Review `bun.lock` and
`backend/go.sum`: they pick the dependency versions that run.

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
- Planned, High from their first commit: `backend/internal/store/**` (schema
  and migrations), `openapi.json` and `backend/internal/api/**` (the public API
  contract), `web/package.json` and `web/bun.lock` (the dependencies that
  run), `deploy/**` and any `Dockerfile` (what is released and how).

**Low**

- `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and `docs/**` other
  than the High and Medium paths.
- `.github/ISSUE_TEMPLATE/**`, `.github/PULL_REQUEST_TEMPLATE.md`.
- `**/*_test.go` and `**/*.test.ts`.

**Medium**

- Everything not listed in High or Low: for example the other Go files under
  `backend/`, and `biome.json`.
- `docs/architecture.md` and `docs/decisions.md`: agents and contributors act
  on them, so a wrong statement there misdirects real work.
- `**/testdata/**` and `**/fixtures/**`: a sample is data entering the
  repository, so its origin gets a real review, and a golden file is the
  parser's behaviour written down.
