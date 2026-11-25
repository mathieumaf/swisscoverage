-- Every query the store runs. `go generate ./...` turns this file into the
-- Go code of the query package; store.go is the only caller of that package.

-- name: LockImport :exec
-- One import at a time. The lock is released when the transaction ends.
SELECT pg_advisory_xact_lock(7765190042);

-- name: ListSnapshots :many
SELECT published_at, sha256, sites, loaded_at
FROM snapshot
ORDER BY published_at;

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
