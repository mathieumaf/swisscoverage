# Contributing

Thanks for looking at swisscoverage. Bug reports, fixes, documentation, data
corrections and focused features are all welcome. For anything larger than a
fix, open an issue first so we can agree on the shape before you write it.

The project is young and most of it is still planned. The
[roadmap](README.md#roadmap) and [docs/architecture.md](docs/architecture.md)
say what exists and what comes next.

Everything in the repository is written in English: code, comments,
documentation, issues and pull requests.

## Setup

You need [Go](https://go.dev) 1.27.4 or later for the backend,
[Docker](https://docs.docker.com/get-started/get-docker/) for the database,
and [Bun](https://bun.sh) 1.4.5 to run Biome, the tool that checks Markdown,
JSON and YAML.

```sh
git clone https://github.com/mathieumaf/swisscoverage.git
cd swisscoverage
bun install
docker compose -f deploy/compose.yaml up -d --wait    # the local database
```

The database is PostgreSQL with PostGIS. It listens on this machine only, on
port 54320, and holds nothing that cannot be loaded again:
`docker compose -f deploy/compose.yaml down -v` throws it away.

## Where things go

[docs/architecture.md](docs/architecture.md) explains the layout and has a
section on where a change belongs. [docs/decisions.md](docs/decisions.md) says
why it is built that way, and [docs/roadmap.md](docs/roadmap.md) what comes
next.

The short version: the backend is written in Go under `backend/`. Today it
holds the `swisscoverage` program with its commands in
`backend/cmd/swisscoverage`, and four packages under `backend/internal/`:
`ofcom`, the parser for OFCOM's file; `archive`, which reads the daily
archive; `store`, the database; and `api`, the public API. The website is
planned in TypeScript under `web/`.

The parser never touches the network, the file system, the environment, a
database or the clock: it receives the bytes of a file from the command that
calls it. Code that needs the database belongs in `backend/internal/store`.
The website reaches the data only by calling the public API.

The API's contract is `openapi.json`, at the root, generated from the code of
`backend/internal/api`. After a change to the API, run
`go run ./cmd/swisscoverage openapi > ../openapi.json` in `backend/` and
commit the result with the change; a test fails until you do. The contract
only grows: a change may add an endpoint, a parameter or a field, never
remove or change one that exists, since clients built on it keep running.

Two rules for the database:

- **A change to the schema is a new file** in
  `backend/internal/store/migrations`, numbered after the last one. A file
  that is merged is never edited. The new schema must keep the previous
  version of the program working, so add, with a default, rather than rename
  or remove.
- **Queries are written in `backend/internal/store/queries.sql`**, and
  `go generate ./...` turns them into the Go code of `internal/store/query`.
  Commit that code with the query. It fails when a query does not match the
  schema, which is the point.

## Checks

Run these before you push. CI runs the same commands.

```sh
cd backend
gofmt -l .           # formatting: prints the files to fix, nothing when good
go mod tidy -diff    # go.mod matches the imports: prints nothing when good
go generate ./...    # the generated query code: git status shows no change
go vet ./...
go test ./...        # needs the local database, see below

cd ..
bun run check        # lint and formatting of Markdown, JSON and YAML
```

The tests that need the database are skipped when it is not running, and
`go test` then still succeeds. Start it as shown under Setup, or run
`CI=true go test ./...`, which fails instead of skipping. Each test creates a
database of its own and drops it when it ends.

`gofmt -w .` fixes the formatting of Go files and `bun run format` fixes the
rest.

A change to behaviour comes with a test that fails without it. A parser change
comes with a real sample of the file it reads, kept small, under the
`testdata` directory beside the test.

The parser's result for the sample is recorded in a golden file,
`testdata/mobile-stations.golden.json`. When a change alters that result on
purpose, `go test ./internal/ofcom -update` rewrites the file. Read its diff
before you commit it: it is the change in behaviour.

To try the parser on the real file, download it once and run
`go run ./cmd/swisscoverage parse <file>` in `backend/`. It prints the number
of stations, the counts per operator, kind and technology, and every value the
parser did not recognise.

To load the archive into the local database, in `backend/`:

```sh
export DATABASE_URL=postgres://swisscoverage:swisscoverage@127.0.0.1:54320/swisscoverage
go run ./cmd/swisscoverage migrate    # creates the tables
go run ./cmd/swisscoverage ingest     # loads every snapshot it does not hold yet
go run ./cmd/swisscoverage serve      # the API, on http://127.0.0.1:8080/v1/
```

## Data

Read [docs/data-sources.md](docs/data-sources.md) before touching anything
that reads or adds data. Two rules matter most:

- **Say where data comes from.** A new source needs an issue first, with its
  publisher and its terms of use.
- **Never contribute scraped data.** Data copied from another map, website or
  app is not accepted, even as a test fixture.

## Pull requests

- **One change per pull request.** A fix and an unrelated cleanup are two.
- **Title in [Conventional Commits](https://www.conventionalcommits.org) form**,
  such as `fix: read sites published without a type`. Pull requests are
  squash-merged and the title becomes the commit.
- **Say how you checked it.** The template asks for the commands you ran and
  what they returned.
- **Update the documentation in the same pull request** when you change what it
  describes.

## Licence of contributions

swisscoverage is licensed under the
[GNU Affero General Public License v3.0](LICENSE). By opening a pull request you
agree that your contribution is published under the same licence. There is no
contributor agreement to sign.

## Conduct and security

Everyone taking part is expected to follow the
[code of conduct](CODE_OF_CONDUCT.md). To report a security problem, follow
[SECURITY.md](SECURITY.md) and do not open a public issue.
