-- Every query the store runs. `go generate ./...` turns this file into the
-- Go code of the query package; the store package is its only caller.

-- name: LockImport :exec
-- One import at a time. The lock is released when the transaction ends.
SELECT pg_advisory_xact_lock(7765190042);

-- name: ListSnapshots :many
SELECT published_at, sha256, sites, loaded_at
FROM snapshot
ORDER BY published_at;

-- name: CountCurrentStations :one
-- The sites that the latest snapshot loaded shows.
SELECT count(*) FROM station_state WHERE valid_to IS NULL;

-- name: InsertSnapshot :exec
INSERT INTO snapshot (published_at, sha256, sites)
VALUES (@published_at, @sha256, @sites);

-- name: ClearIncomingStations :exec
TRUNCATE incoming_station;

-- name: CopyIncomingStations :copyfrom
INSERT INTO incoming_station (
    name, operator, code, east, north,
    kind, power_class, technologies, adaptive, permit_type, permit_date,
    installation_limit_value
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
);

-- name: InsertNewStations :execrows
-- A label at a position that was never seen is a new site.
INSERT INTO station (name, operator, code, east, north, first_seen)
SELECT i.name, i.operator, i.code, i.east, i.north, @published_at
FROM incoming_station i
WHERE NOT EXISTS (
    SELECT 1
    FROM station s
    WHERE s.name = i.name AND s.east = i.east AND s.north = i.north
);

-- name: CloseOutdatedStates :execrows
-- Ends the current state of every site that left the file, or whose
-- published values are no longer the ones recorded.
UPDATE station_state st
SET valid_to = @published_at
FROM station s
LEFT JOIN incoming_station i
    ON i.name = s.name AND i.east = s.east AND i.north = s.north
WHERE st.station_id = s.id
  AND st.valid_to IS NULL
  AND (
      i.name IS NULL
      OR (st.kind, st.power_class, st.technologies, st.adaptive,
          st.permit_type, st.permit_date, st.installation_limit_value)
         IS DISTINCT FROM
         (i.kind, i.power_class, i.technologies, i.adaptive,
          i.permit_type, i.permit_date, i.installation_limit_value)
  );

-- name: OpenNewStates :execrows
-- Starts a state for every site of the file that has no current one: new
-- sites, sites that came back, and sites whose state was just ended.
INSERT INTO station_state (
    station_id, valid_from, kind, power_class, technologies, adaptive,
    permit_type, permit_date, installation_limit_value
)
SELECT s.id, @published_at, i.kind, i.power_class, i.technologies, i.adaptive,
       i.permit_type, i.permit_date, i.installation_limit_value
FROM incoming_station i
JOIN station s
    ON s.name = i.name AND s.east = i.east AND s.north = i.north
WHERE NOT EXISTS (
    SELECT 1
    FROM station_state st
    WHERE st.station_id = s.id AND st.valid_to IS NULL
);

-- name: ListChanges :many
-- The sites that are not the same at two moments: published at one and not
-- at the other, or published at both with different values. A site that
-- changed and changed back in between is not listed.
WITH before AS (
    SELECT st.*
    FROM station_state st
    WHERE st.valid_from <= @from_time
      AND (st.valid_to IS NULL OR st.valid_to > @from_time)
), after AS (
    SELECT st.*
    FROM station_state st
    WHERE st.valid_from <= @to_time
      AND (st.valid_to IS NULL OR st.valid_to > @to_time)
)
SELECT s.name, s.operator, s.code, s.east, s.north,
       (CASE
            WHEN b.id IS NULL THEN 'appeared'
            WHEN a.id IS NULL THEN 'disappeared'
            ELSE 'changed'
        END)::text AS change
FROM station s
LEFT JOIN before b ON b.station_id = s.id
LEFT JOIN after a ON a.station_id = s.id
WHERE (b.id IS NULL AND a.id IS NOT NULL)
   OR (b.id IS NOT NULL AND a.id IS NULL)
   OR (b.id IS NOT NULL AND a.id IS NOT NULL
       AND (b.kind, b.power_class, b.technologies, b.adaptive,
            b.permit_type, b.permit_date, b.installation_limit_value)
           IS DISTINCT FROM
           (a.kind, a.power_class, a.technologies, a.adaptive,
            a.permit_type, a.permit_date, a.installation_limit_value))
ORDER BY s.name, s.east, s.north;

-- The queries below are the API's. They only read, and each request runs
-- them in one read-only transaction, so that it sees one snapshot.

-- name: LatestSnapshot :many
-- The snapshot whose stations are published now. No row before the first
-- import.
SELECT published_at, sha256, sites
FROM snapshot
ORDER BY published_at DESC
LIMIT 1;

-- name: ListSnapshotsPage :many
-- The snapshots, newest first, with how many there are in all.
SELECT published_at, sha256, sites, count(*) OVER () AS total
FROM snapshot
ORDER BY published_at DESC
LIMIT @row_limit OFFSET @row_offset;

-- name: ListCurrentStations :many
-- Every site the latest snapshot shows, with what it shows of it.
SELECT sqlc.embed(cs)
FROM current_station cs
ORDER BY cs.name, cs.east, cs.north;

-- name: SearchStationsInBox :many
-- The sites the latest snapshot shows inside a rectangle in WGS84, edges
-- included, that match every filter. An empty filter matches every site; a
-- site matches the technologies when it has one of them.
SELECT sqlc.embed(cs), count(*) OVER () AS total
FROM station s
JOIN current_station cs ON cs.id = s.id
-- The index finds the boxes that overlap, which it rounds outward; the
-- coordinates then decide exactly. A rectangle may be a line or a point.
WHERE s.position && ST_MakeEnvelope(@west::float8, @south::float8, @east::float8, @north::float8, 4326)
  AND ST_X(s.position) BETWEEN @west::float8 AND @east::float8
  AND ST_Y(s.position) BETWEEN @south::float8 AND @north::float8
  AND (cardinality(@operators::text[]) = 0 OR cs.operator = ANY (@operators::text[]))
  AND (cardinality(@kinds::text[]) = 0 OR cs.kind = ANY (@kinds::text[]))
  AND (cardinality(@power_classes::text[]) = 0 OR cs.power_class = ANY (@power_classes::text[]))
  AND (cardinality(@technologies::text[]) = 0 OR cs.technologies && @technologies::text[])
ORDER BY cs.name, cs.east, cs.north
LIMIT @row_limit OFFSET @row_offset;

-- name: SearchStationsAround :many
-- The sites the latest snapshot shows within a distance of a point in WGS84,
-- nearest first, that match every filter as in SearchStationsInBox.
-- Distances are in metres, on the ellipsoid.
SELECT sqlc.embed(cs),
       ST_Distance(s.position::geography, center.point)::float8 AS distance,
       count(*) OVER () AS total
FROM station s
JOIN current_station cs ON cs.id = s.id
CROSS JOIN (
    SELECT ST_SetSRID(ST_MakePoint(@longitude::float8, @latitude::float8), 4326)::geography AS point
) center
WHERE ST_DWithin(s.position::geography, center.point, @radius::float8)
  AND (cardinality(@operators::text[]) = 0 OR cs.operator = ANY (@operators::text[]))
  AND (cardinality(@kinds::text[]) = 0 OR cs.kind = ANY (@kinds::text[]))
  AND (cardinality(@power_classes::text[]) = 0 OR cs.power_class = ANY (@power_classes::text[]))
  AND (cardinality(@technologies::text[]) = 0 OR cs.technologies && @technologies::text[])
ORDER BY distance, cs.name, cs.east, cs.north
LIMIT @row_limit OFFSET @row_offset;

-- name: GetStation :many
-- One site, found by its label and its position as published.
SELECT s.id, s.name, s.operator, s.code, s.east, s.north,
       ST_Y(s.position)::float8 AS latitude, ST_X(s.position)::float8 AS longitude,
       s.first_seen,
       EXISTS (
           SELECT 1 FROM station_state st WHERE st.station_id = s.id AND st.valid_to IS NULL
       ) AS published
FROM station s
WHERE s.name = @name AND s.east = @east AND s.north = @north;

-- name: ListStationStates :many
-- What was published of one site, period by period, the latest first, with
-- how many periods there are in all.
SELECT valid_from, valid_to, kind, power_class, technologies, adaptive,
       permit_type, permit_date, installation_limit_value,
       count(*) OVER () AS total
FROM station_state
WHERE station_id = @station_id
ORDER BY valid_from DESC
LIMIT @row_limit OFFSET @row_offset;

-- name: CountCurrentStationsBy :many
-- How many sites the latest snapshot shows for each operator, kind, power
-- class and technology. Values no site has are absent. A site counts once
-- for each of its technologies.
SELECT 'operator'::text AS dimension, s.operator AS value, count(*) AS stations
FROM station s
JOIN station_state st ON st.station_id = s.id AND st.valid_to IS NULL
GROUP BY s.operator
UNION ALL
SELECT 'kind'::text, st.kind, count(*)
FROM station_state st
WHERE st.valid_to IS NULL
GROUP BY st.kind
UNION ALL
SELECT 'power_class'::text, st.power_class, count(*)
FROM station_state st
WHERE st.valid_to IS NULL
GROUP BY st.power_class
UNION ALL
SELECT 'technology'::text, technology, count(*)
FROM station_state st, unnest(st.technologies) AS technology
WHERE st.valid_to IS NULL
GROUP BY technology;
