-- What the latest snapshot shows of each site it publishes: the site, its
-- current state, and its position in WGS84, in degrees. The queries of the
-- API read the sites through it, so that the columns are listed once.
--
-- It has no position as a geometry: a query that searches by position joins
-- station for it, so that the spatial indexes, which are on station, apply.
-- PostgreSQL 18 sees that the view joins station too, on the same key, and
-- keeps one join: the plans are those of the queries written in full.
--
-- PostgreSQL refuses to drop or change a column of station or station_state
-- that the view reads: a migration that does must replace the view too.
CREATE VIEW current_station AS
SELECT s.id, s.name, s.operator, s.code, s.east, s.north,
       ST_Y(s.position)::float8 AS latitude, ST_X(s.position)::float8 AS longitude,
       s.first_seen,
       st.kind, st.power_class, st.technologies, st.adaptive,
       st.permit_type, st.permit_date, st.installation_limit_value
FROM station s
JOIN station_state st ON st.station_id = s.id AND st.valid_to IS NULL;
