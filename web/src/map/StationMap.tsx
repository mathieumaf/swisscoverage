import {
  AttributionControl,
  type ExpressionSpecification,
  type GeoJSONSource,
  GeolocateControl,
  Map as MapLibreMap,
  NavigationControl,
  type StyleSpecification,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import { formatNumber, type Locale } from "../i18n/locales";
import { translate, translator } from "../i18n/translate";
import { loadBaseMap } from "./basemap";
import { FilterChips } from "./FilterChips";
import {
  allFilters,
  type Filters,
  hasMapPosition,
  including,
  isEveryStationShown,
  matches,
  readFilters,
  writeFilters,
} from "./filters";
import { MapPanel } from "./MapPanel";
import "./map.css";
import { type Place, placeAttribution } from "./places";
import { StationChoice } from "./StationChoice";
import { StationSheet } from "./StationSheet";
import {
  isStationId,
  localizeLabel,
  type MapStation,
  operatorColors,
  type Station,
  stationsAt,
  toMapData,
  toStations,
} from "./stations";

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

// The zoom at which the map shows an address found by the search, and the
// largest at which it fits a postcode or a municipality.
const addressZoom = 17;
const areaMaxZoom = 15;

// The room the panel takes: at the left of a large screen, its width and
// margins; at the bottom of a phone, about this share of the height.
const panelWidth = 408;
const phonePanelShare = 0.45;

function isPhone(): boolean {
  return window.matchMedia("(max-width: 640px)").matches;
}

// How far from the middle of the map a point must be drawn to be seen beside
// the panel, in pixels.
function besidePanel(): [number, number] {
  return isPhone()
    ? [0, (-window.innerHeight * phonePanelShare) / 2]
    : [panelWidth / 2, 0];
}

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
  // The list a sheet was chosen from, to go back to it.
  const [choice, setChoice] = useState<MapStation[]>([]);
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
  const [namedPosition] = useState(() => hasMapPosition(window.location.hash));
  // Whether the station the address names was opened, or found missing: the
  // address keeps it until then.
  const [restored, setRestored] = useState(false);
  const [sourceReady, setSourceReady] = useState(false);
  const shown = useMemo(
    () => stations.filter((station) => matches(station, filters)),
    [stations, filters],
  );
  const t = translator(locale);

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

    // When the base map's style fails, the map is drawn on a plain
    // background and the stations stay on screen.
    loadBaseMap().then(({ style, failed }) => {
      if (!cancelled) {
        instance = start(style, failed);
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
      // Compact, behind a button, only on a small map. swisstopo's credit,
      // for the search by address, is there from the start: an attribution
      // that changes unfolds over the controls of a small map.
      instance.addControl(
        new AttributionControl({
          customAttribution: [
            translate(locale, "footer.source"),
            placeAttribution,
          ],
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
            "circle-color": "#16181c",
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
              6.5,
              16,
              10,
            ],
            "circle-stroke-color": "#ffffff",
            "circle-stroke-width": [
              "interpolate",
              ["linear"],
              ["zoom"],
              8,
              1,
              14,
              2.5,
            ],
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
            "circle-stroke-color": "#16181c",
            "circle-stroke-width": 3.5,
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
          setChoice([]);
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

  // Keep the station whose sheet is open in the address, once the one it
  // named is restored.
  const [only] = selected;
  const openId = selected.length === 1 ? only?.id : undefined;
  useEffect(() => {
    if (!restored) {
      return;
    }
    updateAddress((search) => {
      if (openId === undefined) {
        search.delete("station");
      } else {
        search.set("station", openId);
      }
    });
  }, [openId, restored]);

  // On a small map MapLibre shows its attribution unfolded until the map is
  // dragged, over the bottom of the panel: once the stations show, it folds,
  // as a drag would. Its button unfolds it again.
  useEffect(() => {
    if (status !== "loading" && isPhone()) {
      container.current
        ?.querySelector(".maplibregl-compact-show")
        ?.classList.remove("maplibregl-compact-show");
    }
  }, [status]);

  // Open the station the address names, once the stations are loaded, and
  // show it unless the address also gives the map's position.
  const opened = useRef(false);
  useEffect(() => {
    if (opened.current || stations.length === 0) {
      return;
    }
    opened.current = true;
    setRestored(true);
    const id = namedStation;
    if (id === null || !isStationId(id)) {
      return;
    }
    const station = stations.find((candidate) => candidate.id === id);
    // A station no longer published still has a sheet, read from the API.
    setSelected([station ?? { id, name: "", operator: "other" }]);
    if (station) {
      setFilters((current) => including(current, station));
      if (!namedPosition) {
        map.current?.easeTo({
          center: station.coordinates,
          zoom: stationZoom,
          offset: besidePanel(),
          duration: 0,
        });
      }
    }
  }, [stations, namedStation, namedPosition]);

  // A station found by its label: show it, even if the filters hid it, and
  // open its sheet.
  const find = useCallback((station: Station) => {
    setFilters((current) => including(current, station));
    map.current?.flyTo({
      center: station.coordinates,
      zoom: stationZoom,
      offset: besidePanel(),
    });
    setChoice([]);
    setSelected([station]);
  }, []);

  // A place found by the search: show it, and mark an address with a point.
  const showPlace = useCallback((place: Place) => {
    const instance = map.current;
    if (!instance) {
      return;
    }
    // The place takes the panel: the open station closes.
    setChoice([]);
    setSelected([]);
    const data = {
      type: "FeatureCollection" as const,
      features:
        place.origin === "address"
          ? [
              {
                type: "Feature" as const,
                geometry: { type: "Point" as const, coordinates: place.center },
                properties: {},
              },
            ]
          : [],
    };
    const source = instance.getSource<GeoJSONSource>("place");
    if (source) {
      source.setData(data);
    } else {
      instance.addSource("place", { type: "geojson", data });
      // A halo around a dot, unlike the stations' coloured points.
      instance.addLayer({
        id: "place-halo",
        type: "circle",
        source: "place",
        paint: {
          "circle-color": "rgba(15, 23, 42, 0.18)",
          "circle-radius": 18,
          "circle-stroke-color": "#0f172a",
          "circle-stroke-width": 1,
        },
      });
      instance.addLayer({
        id: "place",
        type: "circle",
        source: "place",
        paint: {
          "circle-color": "#0f172a",
          "circle-radius": 5,
          "circle-stroke-color": "#ffffff",
          "circle-stroke-width": 2,
        },
      });
    }
    // The place is framed beside the panel: at its right on a large screen,
    // above it on a phone.
    const padding = isPhone()
      ? {
          top: 120,
          bottom: window.innerHeight * phonePanelShare + 24,
          left: 24,
          right: 24,
        }
      : { top: 80, bottom: 48, left: panelWidth + 48, right: 80 };
    // The padding frames this view only: given to `flyTo`, it would stay
    // for every later move.
    const zoom = place.origin === "address" ? addressZoom : areaMaxZoom;
    const camera = instance.cameraForBounds(
      place.bounds ?? [place.center, place.center],
      { padding, maxZoom: zoom },
    );
    instance.flyTo(camera ?? { center: place.center, zoom });
  }, []);

  const clearPlace = useCallback(() => {
    map.current
      ?.getSource<GeoJSONSource>("place")
      ?.setData({ type: "FeatureCollection", features: [] });
  }, []);

  const close = useCallback(() => {
    setChoice([]);
    setSelected([]);
  }, []);

  const filtered = !isEveryStationShown(filters);
  const detail =
    selected.length === 1 && only ? (
      <StationSheet
        key={only.id}
        id={only.id}
        locale={locale}
        apiUrl={apiUrl}
        onClose={close}
        back={
          choice.length > 1
            ? {
                label: t("choice.title", {
                  count: formatNumber(locale, choice.length),
                }),
                onBack: () => {
                  setSelected(choice);
                  setChoice([]);
                },
              }
            : undefined
        }
      />
    ) : selected.length > 1 ? (
      <StationChoice
        stations={selected}
        locale={locale}
        onChoose={(station) => {
          setChoice(selected);
          setSelected([station]);
        }}
        onClose={close}
      />
    ) : null;

  return (
    <div className="station-map absolute inset-0">
      {/* `absolute!`: MapLibre's own stylesheet, outside Tailwind's layers,
          makes its container `position: relative`, and would win otherwise. */}
      <section
        ref={container}
        className="absolute! inset-0"
        aria-label={t("map.label")}
      />
      {/* The search shows once the map can move to what it finds. */}
      {sourceReady && (
        <MapPanel
          locale={locale}
          stations={stations}
          onFind={find}
          onPlace={showPlace}
          onClearPlace={clearPlace}
          detail={detail}
          detailKey={
            selected.map((station) => station.id).join(" ") || undefined
          }
        />
      )}
      {stations.length > 0 && (
        <FilterChips locale={locale} filters={filters} onFilters={setFilters} />
      )}
      {(status !== "ready" || filtered) && (
        <div className="absolute bottom-6 left-[calc(50%+12.75rem)] z-[2] flex max-w-[calc(100%-27rem)] -translate-x-1/2 flex-wrap items-center justify-center gap-x-1 rounded-[1.5rem] bg-surface py-1 pr-1 pl-4 text-[0.875rem] font-semibold shadow-float phone:top-[7.75rem] phone:bottom-auto phone:left-1/2 phone:max-w-[calc(100%-1.5rem)]">
          {status !== "ready" ? (
            <p
              className={`py-1.5 pr-3 ${status === "loading" ? "text-foreground" : "text-danger"}`}
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
          ) : (
            <>
              <p role="status" className="py-1.5 whitespace-nowrap">
                {t("filters.count", {
                  shown: formatNumber(locale, shown.length),
                  total: formatNumber(locale, stations.length),
                })}
              </p>
              <button
                type="button"
                className="min-h-9 cursor-pointer rounded-full bg-field px-3 font-bold whitespace-nowrap text-accent"
                onClick={() => setFilters(allFilters())}
              >
                {t("filters.reset")}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
