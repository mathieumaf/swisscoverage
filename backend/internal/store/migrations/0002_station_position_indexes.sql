-- Spatial indexes for the API, which searches the sites inside a rectangle in
-- WGS84 and within a distance of a point, in metres on the ellipsoid. Without
-- them, PostgreSQL measures every site, and has no statistics on positions to
-- plan the rest of the query with: it then guesses that almost no site
-- matches, and joins as if that were so.
--
-- The expressions are those of queries.sql: a query uses an index only when
-- it repeats its expression exactly.
CREATE INDEX station_position ON station USING gist (position);
CREATE INDEX station_position_geography ON station USING gist ((position::geography));

-- The statistics of the new indexes, for a database that already holds sites.
-- Afterwards, the server keeps them up to date as the import writes.
ANALYZE station;
