-- What the latest snapshot shows of each site it publishes: the site, its
-- current state, and its position in WGS84, in degrees. The queries of the
-- API read the sites through it, so that the columns are listed once.
--
-- It leaves the position as a geometry out, so that the rows read through it
-- do not carry it: a query that searches by position joins station for it.
-- From PostgreSQL 18, the planner sees that the view joins station on the
-- same key and keeps one join; an older server makes one more lookup by key
-- per site.
--
-- A role given SELECT table by table needs it on this view too.
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
