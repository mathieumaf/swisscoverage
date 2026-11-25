-- The stations OFCOM publishes, and how each of them changed over time.
-- docs/architecture.md describes the model.

-- PostGIS turns the published LV95 positions into WGS84 and answers spatial
-- questions. Creating an extension needs rights the program's role may not
-- have: where that is so, whoever sets the database up creates it first, and
-- this line then does nothing.
CREATE EXTENSION IF NOT EXISTS postgis;

-- One file published by OFCOM. The publication time is what tells two files
-- apart, and what the rest of the model refers to.
CREATE TABLE snapshot (
    published_at timestamptz PRIMARY KEY,
    sha256       text        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    sites        integer     NOT NULL CHECK (sites >= 0),
    loaded_at    timestamptz NOT NULL DEFAULT now()
);

-- A site: what never changes about it. OFCOM publishes no identifier, so a
-- site is its label and its position together. The label alone is not unique.
CREATE TABLE station (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text             NOT NULL CHECK (name <> ''),
    operator   text             NOT NULL
        CHECK (operator IN ('swisscom', 'sunrise', 'salt', 'sbb', 'other')),
    code       text             NOT NULL,
    -- The position as published, in LV95 (EPSG:2056), in metres.
    east       double precision NOT NULL,
    north      double precision NOT NULL,
    -- The same position in WGS84. PostGIS computes it, so that no code path
    -- can store a position that disagrees with the published one.
    position   geometry(Point, 4326) NOT NULL GENERATED ALWAYS AS (
        ST_Transform(ST_SetSRID(ST_MakePoint(east, north), 2056), 4326)
    ) STORED,
    first_seen timestamptz      NOT NULL REFERENCES snapshot (published_at),
    UNIQUE (name, east, north)
);

-- What was known of a site between two snapshots. A row covers the whole
-- period during which nothing changed: it starts with the snapshot that first
-- showed these values and ends with the first snapshot that no longer did.
-- A site that left the file has no row without an end.
CREATE TABLE station_state (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    station_id   bigint      NOT NULL REFERENCES station (id),
    valid_from   timestamptz NOT NULL REFERENCES snapshot (published_at),
    valid_to     timestamptz REFERENCES snapshot (published_at),
    kind         text        NOT NULL
        CHECK (kind IN ('outdoor', 'outdoor_low_power', 'indoor', 'tunnel', 'temporary', 'unknown')),
    power_class  text        NOT NULL
        CHECK (power_class IN ('very_low', 'low', 'medium', 'high', 'unknown')),
    technologies text[]      NOT NULL
        CHECK (technologies <@ ARRAY['2G', '3G', '4G', '5G']),
    -- NULL when the file says something the parser did not recognise.
    adaptive     boolean,
    -- NULL when OFCOM publishes no permit information.
    permit_type  text
        CHECK (permit_type IN ('site_data_sheet', 'exempt')),
    -- The date of a site data sheet. NULL when the published one is not a
    -- date that exists.
    permit_date  date,
    -- In V/m. NULL when absent or left blank.
    installation_limit_value double precision,
    CHECK (valid_to > valid_from),
    CHECK (permit_date IS NULL OR (permit_type IS NOT NULL AND permit_type = 'site_data_sheet')),
    UNIQUE (station_id, valid_from)
);

-- A site has at most one state without an end: the current one.
CREATE UNIQUE INDEX station_state_current ON station_state (station_id)
    WHERE valid_to IS NULL;

-- Where the import puts the stations of the snapshot it is loading, to
-- compare them with what is known. It is emptied before and after each load,
-- and nothing else reads it.
CREATE UNLOGGED TABLE incoming_station (
    name         text             NOT NULL,
    operator     text             NOT NULL,
    code         text             NOT NULL,
    east         double precision NOT NULL,
    north        double precision NOT NULL,
    kind         text             NOT NULL,
    power_class  text             NOT NULL,
    technologies text[]           NOT NULL,
    adaptive     boolean,
    permit_type  text,
    permit_date  date,
    installation_limit_value double precision,
    PRIMARY KEY (name, east, north)
);
