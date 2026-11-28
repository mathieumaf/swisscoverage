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

## Base map (OpenFreeMap)

The background of the map: roads, places, water, forests, relief. The site
draws OFCOM's stations on top of it and reads nothing else from it.

|           |                                                                                                                                                       |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Publisher | OpenFreeMap, run by Hyperknot Software Kft. (Hungary)                                                                                                 |
| Data      | Vector tiles built from OpenStreetMap with the OpenMapTiles schema; Natural Earth relief at small scales                                              |
| Style     | Liberty, `https://tiles.openfreemap.org/styles/liberty`, a fork of OSM Liberty                                                                        |
| Access    | Public instance, no key, no registration, no cookies; no limit on map views                                                                           |
| Terms     | [Terms of service](https://openfreemap.org/tos/) and [privacy policy](https://openfreemap.org/privacy/), read on 26 November 2026                     |
| Licences  | Data © OpenStreetMap contributors; OpenMapTiles schema and styles: code BSD-3-Clause, design CC BY 4.0; Noto Sans fonts: SIL OFL 1.1; Maki icons: CC0 |
| Used by   | `web/src/map/`: the visitor's browser loads the style, the tiles, the fonts and the sprites from `tiles.openfreemap.org`                              |

Attribution to show: "OpenFreeMap © OpenMapTiles Data from OpenStreetMap",
with links. The tiles declare it themselves and MapLibre shows it in the
map's attribution, next to OFCOM's.

What to know:

- **No guarantee.** The public instance is free, financed by donations, and
  may change or stop without notice. OpenFreeMap can also be self-hosted, from
  the images it publishes: that is the way out if the public instance goes.
- **Visitors' requests go to a third party.** Each visitor's browser asks
  OpenFreeMap, through Cloudflare, for the tiles of the area they look at.
  OpenFreeMap's access logs hold no IP address; its error logs may, for seven
  days, and it may log addresses for up to 30 days to handle a security
  incident or abuse.
- **Fetching tiles in bulk is forbidden**, beyond offline downloads of at most
  3,000 tiles. The project downloads none: only visitors' browsers load them,
  as they move the map.

## Search by address (swisstopo)

What turns an address, a postcode or a municipality into a place on the map.
The site moves the map there and reads nothing else from it.

|           |                                                                                                                                                                                                                                                                                             |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Publisher | Federal Office of Topography (swisstopo), for the Federal Spatial Data Infrastructure (FSDI)                                                                                                                                                                                                |
| Service   | Location search of the geo.admin.ch API, `https://api3.geo.admin.ch/rest/services/ech/SearchServer` with `type=locations` ([documentation](https://docs.geo.admin.ch/access-data/search.html))                                                                                              |
| Data      | Three origins only: `address`, the official directory of building addresses; `zipcode`, the official directory of towns and cities; `gg25`, the municipalities of swissBOUNDARIES3D                                                                                                         |
| Access    | Public, free of charge, no key, no registration                                                                                                                                                                                                                                             |
| Terms     | [FSDI general terms of use and operating conditions](https://www.geo.admin.ch/en/general-terms-of-use-fsdi), in force since 1 January 2025, and [swisstopo's terms for free geodata](https://www.swisstopo.admin.ch/en/terms-of-use-free-geodata-and-geoservices), read on 28 November 2026 |
| Licence   | swisstopo's free geodata (OGD): use, redistribution and commercial use allowed; the source must be given                                                                                                                                                                                    |
| Used by   | `web/src/map/places.ts`: the visitor's browser sends what they type in the search to `api3.geo.admin.ch`                                                                                                                                                                                    |

Attribution to show: "© swisstopo", wherever the results of the search are
shown. The site shows it under the places found, and in the map's
attribution.

What to know:

- **Fair use.** The REST services allow 40 requests a minute and 21 million a
  year. The terms count "the integration of geoservices in web applications
  with an average of 20,000 users per day" as fair use, ask to avoid bots
  with high query rates, and may restrict or refuse access in case of
  excessive use. The site asks only once the visitor has typed three
  characters and paused, and asks again only when the text changes.
- **The other origins are left out.** `gazetteer` mixes swissNAMES3D with the
  public transport stops of the Federal Office of Transport, whose terms are
  not read here; `parcel`, `district` and `kantone` are not useful to find a
  place on this map.
- **Visitors' searches go to a third party.** The browser of each visitor
  sends the text they type to the Confederation's servers. The
  [Federal Administration's privacy policy](https://www.admin.ch/gov/en/start/terms-and-conditions.html)
  says that its web pages and web services log the IP address, the date, the
  time and the request, kept as long as their purpose requires; the FSDI
  terms say nothing more about the API. Nothing is sent before the visitor
  types.
- **No guarantee.** The service is provided on a best-effort basis, and
  changes that are not backwards compatible are announced one month in
  advance on the
  [geoadmin-api mailing list](https://groups.google.com/group/geoadmin-api).
  When it fails, the search still finds stations by their label.
- **Labels are HTML.** Each result's `label` carries tags such as `<b>`. The
  site reads it as text, never as HTML.

## Other OFCOM datasets

Published on the same portal and not used yet:

- `ch.bakom.mobilnetz-3g`, `-4g`, `-5g`: availability of mobile networks.
- `ch.bakom.radio-fernsehsender`: radio and television transmitters, updated
  daily.

## Adding a source

Open an issue first. Say who publishes the data, under which terms, how often
it changes, and how it would be kept up to date. A source that needs a login,
an API key or a manual download needs a plan for that before any code.
