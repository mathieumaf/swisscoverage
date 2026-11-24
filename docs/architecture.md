# Architecture

This page says how swisscoverage is put together and where a change belongs.
The project is at its beginning: each part below is marked **exists** or
**planned**. Update this page in the pull request that makes a planned part
real, or that changes the plan.

## Principles

1. **The archive is the source of truth.** Every daily file from OFCOM is
   kept, in a separate repository,
   [swisscoverage-data](https://github.com/mathieumaf/swisscoverage-data): the
   original, and a line-per-site form that git can compare from one day to the
   next. The database is derived from the archive and can be rebuilt from it at
   any time, so a parsing bug never destroys information.
2. **One API, several clients.** The web map and the MCP server read the same
   public HTTP API as everyone else. A feature that the map needs is a feature
   of the API.
3. **Data only from sources we may use.** Official open data, documents
   obtained under the Freedom of Information Act, and contributions whose
   origin is known. Never data scraped from another website or app. See
   [data-sources.md](data-sources.md).
4. **Reproducible by anyone.** A contributor can run the whole stack locally
   with one command, and anyone can host their own instance.

## Components

```
              OFCOM daily file
                     │
                     ▼
   ┌──────────────────────────────────┐
   │ archive   one snapshot per day   │   exists (swisscoverage-data)
   └──────────────────────────────────┘
                     │
                     ▼
   ┌──────────────────────────────────┐
   │ import    parse, diff, load      │   planned (apps/ingest)
   │           uses packages/core     │   core exists
   └──────────────────────────────────┘
                     │
                     ▼
   ┌──────────────────────────────────┐
   │ database  PostgreSQL + PostGIS   │   planned (packages/db)
   └──────────────────────────────────┘
                     │
                     ▼
   ┌──────────────────────────────────┐
   │ API       public HTTP API        │   planned (apps/api)
   └──────────────────────────────────┘
          │                     │
          ▼                     ▼
   ┌─────────────┐      ┌─────────────┐
   │ web map     │      │ MCP server  │      planned (apps/web, in apps/api)
   └─────────────┘      └─────────────┘
```

| Part       | Path                                                                   | State   | Role                                                                                                                                      |
| ---------- | ---------------------------------------------------------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| Archive    | [swisscoverage-data](https://github.com/mathieumaf/swisscoverage-data) | exists  | A separate repository. A scheduled workflow downloads OFCOM's file once a day, checks it against the catalogue's checksum and commits it. |
| Core       | `packages/core`                                                        | exists  | Domain model, and parsers that turn source files into it. Pure TypeScript: no network, no database, no clock.                             |
| Database   | `packages/db`                                                          | planned | Schema, migrations and queries for PostgreSQL with PostGIS. Converts positions from LV95 to WGS84.                                        |
| Import     | `apps/ingest`                                                          | planned | Command-line program that reads a snapshot, compares it with the previous one and loads the result. Runs once a day.                      |
| API        | `apps/api`                                                             | planned | Public HTTP API with an OpenAPI description, and the MCP server built on the same queries.                                                |
| Web map    | `apps/web`                                                             | planned | MapLibre map that reads the API.                                                                                                          |
| Deployment | `deploy/`                                                              | planned | Docker Compose for local work and tests; a Helm chart for Kubernetes in production.                                                       |

## Where a change belongs

- **Understanding a source file** (a new label, a new field, a new dataset):
  `packages/core`, with a test that uses a real sample of the file.
- **Anything that needs the database, the network or the current time**: an
  app, or `packages/db`. Not `packages/core`.
- **A new question the data should answer**: the API first, then the map or the
  MCP server on top of it.

`packages/core` reports what it does not recognise as warnings and keeps going,
so a new label upstream never blocks the daily import. It throws only when the
file is not the dataset at all, for example when coordinates are in another
reference system.

## Stack

- **Language:** TypeScript everywhere, run and tested with Bun.
- **Checks:** Biome for lint and formatting, `tsc` for types, `bun test`.
- **Database:** PostgreSQL with PostGIS.
- **Map:** MapLibre GL.
- **Packaging:** one application image for the API, the MCP server and the web
  map; the daily import runs from the same image with another command.

## Deployment

Planned, and not built yet.

- **Local and tests:** Docker Compose starts the database and the application.
- **Production:** a Helm chart deploys to Kubernetes: the application with
  rolling updates, the daily import as a scheduled job, the database, and the
  ingress with TLS. The public instance is meant to run on a managed cluster in
  Switzerland, and the same chart must work on a self-managed cluster.
- **Releases:** continuous integration builds the image, publishes it to the
  GitHub container registry, and updates the cluster.
