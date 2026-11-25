# Data sources

Which data swisscoverage uses, where it comes from, what it contains and under
which terms. Read this before adding a source or changing a parser.

## The rule

A source is acceptable when we know where it comes from and are allowed to
redistribute it: official open data, documents obtained under the Freedom of
Information Act, or contributions whose origin is documented.

Data taken from another map, website or app by scraping is never acceptable,
whatever its quality. Do not open a pull request that contains any.

## Mobile phone base stations (OFCOM, public)

The base dataset of the project.

|              |                                                                                                                               |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| Publisher    | Federal Office of Communications (OFCOM / BAKOM)                                                                              |
| Dataset      | `ch.bakom.standorte-mobilfunkanlagen`                                                                                         |
| Download     | [data.geo.admin.ch](https://data.geo.admin.ch/browser/index.html#/collections/ch.bakom.standorte-mobilfunkanlagen) (STAC API) |
| Format       | One GeoJSON file of about 28 MB, coordinates in LV95 (EPSG:2056)                                                              |
| Update       | Daily                                                                                                                         |
| Terms        | Open government data: free use, [the source must be given](https://opendata.swiss/en/terms-of-use/#terms_by)                  |
| Explanations | [OFCOM's notes on the overview map](https://www.bakom.admin.ch/en/explanations-on-overview-map)                               |
| Parser       | `backend/internal/ofcom/mobile_stations.go`                                                                                   |

Attribution to use: "Source: Federal Office of Communications OFCOM".

OFCOM replaces the file every day and keeps no history. Every version since
23 November 2026 is kept in
[swisscoverage-data](https://github.com/mathieumaf/swisscoverage-data), which
lists them in `ofcom/mobile-stations/snapshots.ndjson`. The import
(`swisscoverage ingest`) reads that list and loads the original files; it
never downloads from OFCOM itself.

### Fields

Each feature is one site. Every property is a display string, repeated with the
suffixes `_de`, `_fr`, `_it` and `_en`. The parser reads the English ones.

| Property      | Content                                         | Values seen                                                                                                |
| ------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `station`     | Operator and station code                       | `Swisscom DIBH`, `Sunrise ZH601-1`, `Salt LU_0615A`, `SBB AU-WAE-02202`, or a bare code such as `DTM-SCHL` |
| `koord`       | LV95 coordinates as text, equal to the geometry | `2692356,1233045`                                                                                          |
| `typ`         | Kind of site                                    | `Outdoor > 6 Werp`, `Outdoor ≤ 6 Werp`, `Indoor ≤ 6 Werp`, `Tunnel`, `Temporary station`, or empty         |
| `power`       | Power class                                     | very low (up to 6 W), low (up to 500 W), medium (up to 5,000 W), high (over 5,000 W)                       |
| `techno`      | Technologies in service                         | Any combination of 2G, 3G, 4G, 5G                                                                          |
| `adaptiv`     | Adaptive antennas                               | Empty, or `Partially adaptive operation`                                                                   |
| `bewilligung` | Permit information                              | `Site data sheet <date>`, a sentence stating the site is exempt under NISV Annex 1 No. 64, or empty        |
| `agw`         | Installation limit value                        | `Installation limit value 4.0 V/m` (also 5.0 and 6.0), `Installation limit value - V/m`, or empty          |

### What to know before using it

Counts are from the snapshot of 23 November 2026, which held 22,777 sites.

- **One point per site.** The position is that of the first mast. Other masts
  on the same or a neighbouring roof are not published.
- **The power is a class, not a value.** It is the class of ERP90, the highest
  cumulated power in a 90° sector.
- **Sites below 1 W are not published.**
- **The station label is not unique.** One label can appear at several
  positions. The pair of label and position was unique in the snapshot.
- **About a hundred sites have no type.**
- **Not only the three operators.** SBB's railway network accounts for about
  1,200 sites, all 2G. Twenty sites near the border carry a bare code without
  an operator name; the parser groups them under `other`.
- **Technologies are per site**, not per antenna.

### What it does not contain

Frequency bands, the direction of each antenna, exact powers, antenna height,
commissioning dates and cell identifiers.

## OFCOM antenna database (on request)

OFCOM keeps a database of operating data that the operators must report. It
holds what the public file lacks: frequencies, direction, power, antenna
diagram, height and commissioning date, per antenna.

It is not published, but it can be obtained under the Freedom of Information
Act (LTrans / BGÖ):

- The Federal Administrative Court ruled in 2023 that access must be granted
  for 5G sites ([judgment A-516/2022](https://entscheide.weblaw.ch/cache.php?link=12-09-2023-A-516-2022),
  final).
- In June 2025 the Federal Data Protection and Information Commissioner
  recommended access to a wider set of columns
  ([recommendation, in French](https://www.oeffentlichkeitsgesetz.ch/downloads/dokumente/2025/2025_06_06_OFCOM_Banque%20de%20donn%C3%A9es%20antennes%205G.pdf)).
- Documents obtained this way may be used freely, within copyright and data
  protection law
  ([Commissioner's guidance](https://www.edoeb.admin.ch/fr/acces-aux-documents-officiels)).

**Status:** a request for the current extract, all operators and all
technologies, was filed in November 2026. This section will describe the
fields once the data arrives. Version 1.0 depends on it.

## Other OFCOM datasets

Published on the same portal and not used yet:

- `ch.bakom.mobilnetz-3g`, `-4g`, `-5g`: availability of mobile networks.
- `ch.bakom.radio-fernsehsender`: radio and television transmitters, updated
  daily.

## Adding a source

Open an issue first. Say who publishes the data, under which terms, how often
it changes, and how it would be kept up to date. A source that needs a login,
an API key or a manual download needs a plan for that before any code.
