# swisscoverage

An open map, API and MCP server for mobile network antennas in Switzerland,
built on OFCOM's open data. A TypeScript monorepo run with Bun. The project is
at its beginning: `packages/core` exists, the apps are planned.

- `docs/architecture.md`: read before adding a package, an app or a dependency.
  It says what exists, what is planned, and where a change belongs.
- `docs/data-sources.md`: read before touching a parser or adding data. It
  describes each source file and the rule on which data may be used.

Everything in the repository and on GitHub is written in English.

## Proof

Run with Bun 1.4.5 and Node 26. Each command exits 0 when the change is good.

| Command             | Proves                         | Expected                   |
| ------------------- | ------------------------------ | -------------------------- |
| `bun run check`     | lint, formatting, types, tests | prints `0 fail`            |
| `bun run lint`      | lint and formatting only       | prints `No fixes applied.` |
| `bun run typecheck` | types only                     | prints no TypeScript error |
| `bun run test`      | tests only                     | prints `0 fail`            |

`bun run format` fixes formatting and import order.

There is no app to run yet. To see a parser change against real data, parse
today's public file:

```sh
(set -o pipefail; curl -sSf https://data.geo.admin.ch/ch.bakom.standorte-mobilfunkanlagen/standorte-mobilfunkanlagen/standorte-mobilfunkanlagen_2056.json | bun -e 'import { parseMobileStations } from "./packages/core/src/index.ts"; const r = parseMobileStations(await Bun.stdin.json()); console.log(r.stations.length, "stations,", r.warnings.length, "warnings"); console.log(r.warnings.slice(0, 20).join("\n"));')
```

It prints about `23000 stations, 0 warnings` and exits 0. It exits with an
error when the download fails or the file is not the dataset. A warning means
the file contains a label the parser does not know: add it, with a fixture
taken from that file.

Everything above is safe: the download is a public, read-only file, fetched
once, and nothing is written to disk. The repository holds no credentials, no
database and no deployment today.

Off limits:

- Fetching data from any map, website or app other than the sources listed in
  `docs/data-sources.md`, including to build a fixture or to compare results.
- Downloading the OFCOM file in a loop or in tests. Tests read the fixtures.

When a database, an API or a deployment is added, the pull request that adds it
must extend this section: how to run it against throwaway data, and which
commands touch production and are off limits.

## Code Review Rules

Flag a change when:

- **It loses what the source said.** A parser drops a station, or throws,
  because of one label it does not recognise, so a new upstream value blocks
  the whole daily import. The expected behaviour is to keep the station, set
  the field to `unknown` or `null`, and add a warning.
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
- **It breaks the core boundary.** `packages/core` reads the network, the file
  system, the environment, a database or the clock. It must stay pure so its
  tests are deterministic and every app can rely on it.

Skip third-party texts: `LICENSE` and `CODE_OF_CONDUCT.md`. For
`**/fixtures/*.json`, check where the sample comes from, not its formatting.
Review `bun.lock`: it picks the dependency versions that run.

Cite file:line for every finding, and post at most 3 nits.

## Risk

When paths overlap, the higher tier wins.

**High**

- `.github/workflows/**`, `.github/dependabot.yml`: what runs with the
  repository's token, and what may merge.
- `package.json`, `packages/*/package.json`, `apps/*/package.json`, `bun.lock`:
  the dependencies that run.
- `AGENTS.md`, `CLAUDE.md`, `SECURITY.md`, `docs/security/**`: they steer
  review and shipping.
- `docs/data-sources.md`: the list of sources the project may use. Adding one
  there is what allows its data in.
- `LICENSE`: the terms of the whole project.
- Planned, High from their first commit: `packages/db/**` (schema and
  migrations), `apps/api/**` (the public API contract), `deploy/**` and any
  `Dockerfile` (what is released and how).

**Low**

- `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and `docs/**` other
  than the High paths above.
- `.github/ISSUE_TEMPLATE/**`, `.github/PULL_REQUEST_TEMPLATE.md`.
- `**/*.test.ts`.

**Medium**

- Everything not listed in High or Low: for example the source files under
  `packages/core/src/`, `biome.json` and `tsconfig.json`.
- `**/fixtures/**`: a fixture is data entering the repository, so its origin
  gets a real review.
