# swisscoverage

An open map of mobile network antennas in Switzerland, with a public API and an
MCP server over the same data.

> **Status: early development.** Nothing is deployed yet. The repository holds
> the foundations: the data model, the parser for the official dataset, and the
> project's documentation. Follow the [roadmap](#roadmap) to see what comes next.

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

| Version   | Scope                                                                                               |
| --------- | --------------------------------------------------------------------------------------------------- |
| **0.1**   | Map of the public OFCOM dataset with filters; daily archive of the source file; first API endpoints |
| **0.x**   | History and deployment map (what changed, and when); statistics; data exports; MCP server           |
| **1.0**   | Detailed data: frequency bands, directions, per-antenna sheets                                      |
| **Later** | Speed tests and community features                                                                  |

## Repository layout

```
apps/        Deployable programs: API, web map, data import (none yet)
packages/
  core/      Domain model and the parser for the OFCOM dataset
docs/        Architecture and data-source documentation
```

[docs/architecture.md](docs/architecture.md) explains how the pieces fit
together and where a change belongs.

## Development

You need [Bun](https://bun.sh) 1.4.5 and Node 26 (`.nvmrc`).

```sh
git clone https://github.com/mathieumaf/swisscoverage.git
cd swisscoverage
bun install
bun run check    # lint, formatting, types, tests
```

## Contributing

Contributions are welcome: code, documentation, data corrections and ideas.
Start with [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

The code is licensed under the [GNU Affero General Public License v3.0](LICENSE).
If you run a modified version as a network service, you must offer its source
to its users.

The data keeps the terms of its source. See
[docs/data-sources.md](docs/data-sources.md).
