import {
  AttributionControl,
  type ExpressionSpecification,
  type GeoJSONSource,
  GeolocateControl,
  Map as MapLibreMap,
  NavigationControl,
  type StyleSpecification,
  setWorkerUrl,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?url";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import type { Locale } from "../i18n/locales";
import { translate } from "../i18n/translate";
import { FilterPanel } from "./FilterPanel";
import { type Filters, matches, readFilters, writeFilters } from "./filters";
import "./map.css";
import { StationChoice } from "./StationChoice";
import { StationSheet } from "./StationSheet";
import {
  localizeLabel,
  type MapStation,
  operatorColors,
  type Station,
  stationsAt,
  toMapData,
  toStations,
} from "./stations";

// MapLibre draws in a worker, served from this site. Its default address is
// relative to the library, which the bundler moves.
setWorkerUrl(workerUrl);

// The base map: OpenFreeMap's Liberty style (docs/data-sources.md).
const baseMapStyle = "https://tiles.openfreemap.org/styles/liberty";

// What the map is drawn on when the base map's style cannot be loaded: a plain
// background, so that the stations stay on screen.
const fallbackStyle: StyleSpecification = {
  version: 8,
  sources: {},
  layers: [
    {
      id: "background",
      type: "background",
      paint: { "background-color": "#e8eaed" },
    },
  ],
};

// Switzerland, with a margin for the stations near the border.
const switzerland: [[number, number], [number, number]] = [
  [5.8, 45.75],
  [10.6, 47.85],
];

// Stations are grouped up to this zoom; beyond it, each one shows.
const clusterMaxZoom = 8;

// The zoom at which the map shows a station found by its label or named in
// the address.
const stationZoom = 15;

// Rewrites the parameters of the page's address without reloading it, and
// keeps the map's position, which MapLibre writes after `#map=`. Commas stay
// as they are, so that a shared address reads `operator=swisscom,salt`.
function updateAddress(change: (search: URLSearchParams) => void) {
  const url = new URL(window.location.href);
  const search = new URLSearchParams(url.search);
  change(search);
  url.search = search.toString().replaceAll("%2C", ",");
  window.history.replaceState(window.history.state, "", url);
}

const operatorColor: ExpressionSpecification = [
  "match",
  ["get", "operator"],
  ...Object.entries(operatorColors).flat(),
  operatorColors.other,
] as unknown as ExpressionSpecification;

interface Props {
  locale: Locale;
  // The origin of the public API, which the browser calls itself.
  apiUrl: string;
}

// `baseMapError`: the stations are drawn, the background is missing.
type Status = "loading" | "ready" | "error" | "baseMapError";

export default function StationMap({ locale, apiUrl }: Props) {
  const container = useRef<HTMLElement>(null);
  const map = useRef<MapLibreMap | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  // The stations under the last click: one opens its sheet, several are
  // listed to choose from.
  const [selected, setSelected] = useState<MapStation[]>([]);
  // Every station of the latest snapshot, once loaded, and what the visitor
  // chose to see, read first from the address so that a view can be shared.
  const [stations, setStations] = useState<Station[]>([]);
  const [filters, setFilters] = useState<Filters>(() =>
    readFilters(new URLSearchParams(window.location.search)),
  );
  // The station and the position the address names, read once, before this
  // component and MapLibre rewrite the address.
  const [namedStation] = useState(() =>
    new URLSearchParams(window.location.search).get("station"),
  );
  const [namedPosition] = useState(() => window.location.hash.includes("map="));
  const [panelOpen, setPanelOpen] = useState(
    () => !window.matchMedia("(max-width: 640px)").matches,
  );
  const [sourceReady, setSourceReady] = useState(false);
  const shown = useMemo(
    () => stations.filter((station) => matches(station, filters)),
    [stations, filters],
  );
  const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);

  useEffect(() => {
    const element = container.current;
    if (element === null) {
      return;
    }
    let cancelled = false;
    let instance: MapLibreMap | undefined;

    // The stations load while the base map does.
    const api = createApiClient(apiUrl, browserTimeoutMs);
    const stations = api.GET("/v1/stations").then(({ data, response }) => {
      if (data === undefined) {
        throw new Error(`The API answered ${response.status}`);
      }
      return toStations(data);
    });
    stations.catch((cause: unknown) => {
      console.error("Loading the stations failed:", cause);
      if (!cancelled) {
        setStatus("error");
      }
    });

    // The base map's style is read here rather than by MapLibre, which would
    // never finish loading without it: when it fails, the map is drawn on a
    // plain background and the stations stay on screen.
    fetch(baseMapStyle, { signal: AbortSignal.timeout(browserTimeoutMs) })
      .then((response) => {
        if (!response.ok) {
          throw new Error(`The base map answered ${response.status}`);
        }
        return response.json() as Promise<StyleSpecification>;
      })
      .then(
        (style) => ({ style, styleFailed: false }),
        (cause: unknown) => {
          console.error("Loading the base map failed:", cause);
          return { style: fallbackStyle, styleFailed: true };
        },
      )
      .then(({ style, styleFailed }) => {
        if (!cancelled) {
          instance = start(style, styleFailed);
        }
      });

    function start(
      style: StyleSpecification,
      styleFailed: boolean,
    ): MapLibreMap | undefined {
      let instance: MapLibreMap;
      try {
        instance = new MapLibreMap({
          container: element as HTMLElement,
          style,
          bounds: switzerland,
          maxBounds: [
            [3.5, 44.5],
            [13, 49],
          ],
          // The position goes into the address, after `#map=`, so that a
          // view can be shared.
          hash: "map",
          attributionControl: false,
          dragRotate: false,
          pitchWithRotate: false,
        });
      } catch (cause) {
        // Most often a browser without WebGL.
        console.error("Creating the map failed:", cause);
        setStatus("error");
        return undefined;
      }
      map.current = instance;
      let loaded = false;
      let baseMapFailed = styleFailed;
      if (styleFailed) {
        setStatus((current) =>
          current === "error" ? current : "baseMapError",
        );
      }

      instance.touchZoomRotate.disableRotation();
      // Compact, behind a button, only on a small map.
      instance.addControl(
        new AttributionControl({
          customAttribution: translate(locale, "footer.source"),
        }),
      );
      instance.addControl(
        new NavigationControl({ showCompass: false }),
        "top-right",
      );
      // The visitor's position, only when they ask for it.
      instance.addControl(
        new GeolocateControl({
          positionOptions: { enableHighAccuracy: true },
          trackUserLocation: false,
        }),
        "top-right",
      );

      // One tile that fails leaves a hole. Before the map has loaded, any other
      // error means the base map cannot be drawn: its tile source, its fonts or
      // its sprites failed. The stations are still drawn, but the error stays
      // shown.
      instance.on("error", (event) => {
        if (!loaded && !("tile" in event) && !cancelled) {
          baseMapFailed = true;
          console.error("Loading the base map failed:", event.error);
          setStatus((current) =>
            current === "error" ? current : "baseMapError",
          );
          return;
        }
        console.error(event.error);
      });

      instance.on("load", async () => {
        loaded = true;
        for (const layer of instance.getStyle().layers) {
          if (layer.type === "symbol" && layer.layout?.["text-field"]) {
            instance.setLayoutProperty(
              layer.id,
              "text-field",
              localizeLabel(layer.layout["text-field"], locale),
            );
          }
        }
        instance.addSource("stations", {
          type: "geojson",
          data: { type: "FeatureCollection", features: [] },
          cluster: true,
          clusterMaxZoom,
          clusterRadius: 50,
        });
        instance.addLayer({
          id: "clusters",
          type: "circle",
          source: "stations",
          filter: ["has", "point_count"],
          paint: {
            "circle-color": "#1e293b",
            "circle-opacity": 0.85,
            "circle-radius": [
              "step",
              ["get", "point_count"],
              14,
              50,
              18,
              250,
              23,
              1000,
              29,
            ],
            "circle-stroke-color": "#ffffff",
            "circle-stroke-width": 2,
          },
        });
        // The counts are written with the base map's fonts, which the plain
        // background does not have.
        if (instance.getStyle().glyphs) {
          instance.addLayer({
            id: "cluster-counts",
            type: "symbol",
            source: "stations",
            filter: ["has", "point_count"],
            layout: {
              "text-field": ["get", "point_count_abbreviated"],
              "text-font": ["Noto Sans Bold"],
              "text-size": 12,
              "text-allow-overlap": true,
            },
            paint: { "text-color": "#ffffff" },
          });
        }
        instance.addLayer({
          id: "stations",
          type: "circle",
          source: "stations",
          filter: ["!", ["has", "point_count"]],
          paint: {
            "circle-color": operatorColor,
            "circle-radius": [
              "interpolate",
              ["linear"],
              ["zoom"],
              8,
              4,
              12,
              6,
              16,
              9,
            ],
            "circle-stroke-color": "#ffffff",
            "circle-stroke-width": 1.5,
          },
        });
        instance.addLayer({
          id: "selected",
          type: "circle",
          source: "stations",
          filter: ["in", ["get", "id"], ["literal", []]],
          paint: {
            "circle-color": "rgba(0, 0, 0, 0)",
            "circle-radius": [
              "interpolate",
              ["linear"],
              ["zoom"],
              8,
              9,
              12,
              11,
              16,
              15,
            ],
            "circle-stroke-color": "#0f172a",
            "circle-stroke-width": 3,
          },
        });

        try {
          const data = await stations;
          if (!cancelled) {
            setStations(data);
            setSourceReady(true);
            setStatus(baseMapFailed ? "baseMapError" : "ready");
          }
        } catch {
          // Already reported when the request failed.
        }
      });

      instance.on("click", "clusters", async (event) => {
        const feature = event.features?.[0];
        const clusterId = feature?.properties?.cluster_id;
        if (
          feature?.geometry.type !== "Point" ||
          typeof clusterId !== "number"
        ) {
          return;
        }
        try {
          const zoom = await instance
            .getSource<GeoJSONSource>("stations")
            ?.getClusterExpansionZoom(clusterId);
          if (!cancelled) {
            instance.easeTo({
              center: feature.geometry.coordinates as [number, number],
              zoom: Math.min(
                zoom ?? instance.getZoom() + 2,
                clusterMaxZoom + 1,
              ),
            });
          }
        } catch {
          // The stations changed while the cluster was being read: no zoom.
        }
      });
      instance.on("click", "stations", (event) => {
        const found = stationsAt(event.features ?? []);
        if (found.length > 0) {
          setSelected(found);
        }
      });
      for (const layer of ["clusters", "stations"]) {
        instance.on("mouseenter", layer, () => {
          instance.getCanvas().style.cursor = "pointer";
        });
        instance.on("mouseleave", layer, () => {
          instance.getCanvas().style.cursor = "";
        });
      }

      return instance;
    }

    return () => {
      cancelled = true;
      map.current = null;
      instance?.remove();
    };
  }, [apiUrl, locale]);

  // Outline the stations whose sheet or list is open.
  useEffect(() => {
    const instance = map.current;
    if (instance?.getLayer("selected")) {
      instance.setFilter("selected", [
        "in",
        ["get", "id"],
        ["literal", selected.map((station) => station.id)],
      ]);
    }
  }, [selected]);

  // Draw the stations the filters keep.
  useEffect(() => {
    if (sourceReady) {
      map.current
        ?.getSource<GeoJSONSource>("stations")
        ?.setData(toMapData(shown));
    }
  }, [shown, sourceReady]);

  // Keep the filters in the address.
  useEffect(() => {
    updateAddress((search) => writeFilters(filters, search));
  }, [filters]);

  // Keep the station whose sheet is open in the address.
  const [only] = selected;
  const openId = selected.length === 1 ? only?.id : undefined;
  useEffect(() => {
    updateAddress((search) => {
      if (openId === undefined) {
        search.delete("station");
      } else {
        search.set("station", openId);
      }
    });
  }, [openId]);

  // Open the station the address names, once the stations are loaded, and
  // show it unless the address also gives the map's position.
  const opened = useRef(false);
  useEffect(() => {
    if (opened.current || stations.length === 0) {
      return;
    }
    opened.current = true;
    const id = namedStation;
    if (id === null || id === "") {
      return;
    }
    const station = stations.find((candidate) => candidate.id === id);
    // A station no longer published still has a sheet, read from the API.
    setSelected([station ?? { id, name: "", operator: "other" }]);
    if (station && !namedPosition) {
      map.current?.jumpTo({ center: station.coordinates, zoom: stationZoom });
    }
  }, [stations, namedStation, namedPosition]);

  // A station found by its label: show it and open its sheet.
  const find = useCallback((station: Station) => {
    map.current?.flyTo({ center: station.coordinates, zoom: stationZoom });
    setSelected([station]);
    if (window.matchMedia("(max-width: 640px)").matches) {
      setPanelOpen(false);
    }
  }, []);

  const close = useCallback(() => setSelected([]), []);

  return (
    <div className="station-map absolute inset-0">
      {/* `absolute!`: MapLibre's own stylesheet, outside Tailwind's layers,
          makes its container `position: relative`, and would win otherwise. */}
      <section
        ref={container}
        className="absolute! inset-0"
        aria-label={t("map.label")}
      />
      {status !== "ready" && (
        <p
          className={`absolute bottom-10 left-1/2 max-w-[calc(100%-1.5rem)] -translate-x-1/2 rounded-full bg-background px-4 py-2 text-center text-[0.875rem] shadow-[0_2px_8px_rgb(0_0_0/0.2)] ${status === "loading" ? "text-foreground" : "text-danger"}`}
          role="status"
        >
          {
            {
              loading: t("map.loading"),
              error: t("map.error"),
              baseMapError: t("map.baseMapError"),
            }[status]
          }
        </p>
      )}
      {stations.length > 0 && (
        <FilterPanel
          locale={locale}
          filters={filters}
          onFilters={setFilters}
          stations={stations}
          shown={shown.length}
          onFind={find}
          open={panelOpen}
          onOpen={setPanelOpen}
        />
      )}
      {selected.length === 1 && only && (
        <StationSheet
          key={only.id}
          id={only.id}
          locale={locale}
          apiUrl={apiUrl}
          onClose={close}
        />
      )}
      {selected.length > 1 && (
        <StationChoice
          stations={selected}
          locale={locale}
          onChoose={(station) => setSelected([station])}
          onClose={close}
        />
      )}
    </div>
  );
}
