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

You need [Bun](https://bun.sh) 1.4.5 and Node 26 (`.nvmrc`).

```sh
git clone https://github.com/mathieumaf/swisscoverage.git
cd swisscoverage
bun install
```

## Where things go

[docs/architecture.md](docs/architecture.md) explains the layout and has a
section on where a change belongs. [docs/decisions.md](docs/decisions.md) says
why it is built that way, and [docs/roadmap.md](docs/roadmap.md) what comes
next.

The short version: today the only code is the parser in `packages/core`, plain
TypeScript with its tests beside it. The backend is planned in Go under
`backend/` and the website in TypeScript under `web/`. The parser never touches
the network, the database or the clock; whatever does belongs in the backend.

## Checks

Run this before you push. CI runs the same command.

```sh
bun run check    # lint, formatting, types, tests
```

`bun run format` fixes formatting and import order.

A change to behaviour comes with a test that fails without it. A parser change
comes with a real sample of the file it reads, kept small, under a `fixtures`
directory beside the test.

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
