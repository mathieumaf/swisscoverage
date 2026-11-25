# swisscoverage

An open map of mobile network antennas in Switzerland, with a public API and an
MCP server over the same data.

> **Status: early development.** Nothing is deployed yet. The repository holds
> the foundations: the parser for the official dataset, the database and the
> import that loads the daily archive into it, the architecture and the plan.
> Follow the [roadmap](docs/roadmap.md) to see what comes next.

## Why

Where the antennas of Swisscom, Sunrise and Salt stand, and what they transmit,
is public information in Switzerland. It is published as a raw file that few
people can read, and the tools built on it so far are closed.

swisscoverage aims to make that information easy to use, for everyone:

- **A map** to explore sites by operator, technology and type.
- **An API** so the data can be queried without opening the map.
- **An MCP server** so AI assistants can answer questions from the same data.
- **Open data**: every daily snapshot is
  [archived](https://github.com/mathieumaf/swisscoverage-data), so the history of the
  network can be studied and reproduced.

The project is independent. It is not affiliated with any operator or with the
Federal Office of Communications.

## Data

The base dataset is the list of mobile phone base stations that the Federal
Office of Communications (OFCOM) publishes every day as open government data:
about 23,000 sites with their operator, position, technologies, type and power
class.

Frequency bands, antenna directions and exact powers are not in that file. They
exist in OFCOM's antenna database and can be obtained under the Swiss Freedom of
Information Act; a request has been filed.

[docs/data-sources.md](docs/data-sources.md) describes each source, its
licence, its fields and its quirks.

Source of the antenna data: Federal Office of Communications OFCOM.

## Roadmap

| Version   | Scope                                                                       |
| --------- | --------------------------------------------------------------------------- |
| **0.1**   | The public OFCOM dataset on a map with filters, behind a public API         |
| **0.2**   | MCP server; history and deployment map (what changed, and when); statistics |
| **1.0**   | Detailed data: frequency bands, directions, per-antenna sheets              |
| **Later** | Estimated coverage, speed tests and community features                      |

[docs/roadmap.md](docs/roadmap.md) breaks this down into steps and says where
the project stands.

## How it is built

- **Backend in Go**: the public API, the daily import and the MCP server.
- **Website in TypeScript**: the map and the pages, a client of the API like
  any other.
- **PostgreSQL with PostGIS** for the spatial queries and the history.
- **Kubernetes** in production, described by a Helm chart anyone can reuse.

Most of this is planned, not built. Today the repository holds:

```
backend/     The `swisscoverage` program, in Go: the parser for the OFCOM
             dataset, the database schema, and the import that records what
             changed from one day to the next
deploy/      Docker Compose with the database, for local work and tests
docs/        Architecture, decisions, roadmap, data sources, security
```

- [docs/architecture.md](docs/architecture.md): how the pieces fit together
  and where a change belongs.
- [docs/decisions.md](docs/decisions.md): why each choice was made, and what
  was rejected.

## Development

You need [Go](https://go.dev) 1.27.4 or later, Docker for the database, and
[Bun](https://bun.sh) 1.4.5 for the tool that checks the documentation.

```sh
git clone https://github.com/mathieumaf/swisscoverage.git
cd swisscoverage
docker compose -f deploy/compose.yaml up -d --wait    # PostgreSQL with PostGIS

cd backend
go test ./...                                         # the backend's tests
export DATABASE_URL=postgres://swisscoverage:swisscoverage@127.0.0.1:54320/swisscoverage
go run ./cmd/swisscoverage migrate                    # creates the tables
go run ./cmd/swisscoverage ingest                     # loads the archive

cd ..
bun install
bun run check                                         # Markdown, JSON and YAML
```

[CONTRIBUTING.md](CONTRIBUTING.md) lists every check that CI runs.

## Contributing

Contributions are welcome: code, documentation, data corrections and ideas.
Start with [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

The code is licensed under the [GNU Affero General Public License v3.0](LICENSE).
If you run a modified version as a network service, you must offer its source
to its users.

The data keeps the terms of its source. See
[docs/data-sources.md](docs/data-sources.md).
