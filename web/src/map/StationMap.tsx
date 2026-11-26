import {
  AttributionControl,
  type ExpressionSpecification,
  type GeoJSONSource,
  GeolocateControl,
  Map as MapLibreMap,
  NavigationControl,
  setWorkerUrl,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?url";
import { useEffect, useRef, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import type { Locale } from "../i18n/locales";
import { translate } from "../i18n/translate";
import "./map.css";
import { StationSheet } from "./StationSheet";
import { localizeLabel, operatorColors, toMapData } from "./stations";

// MapLibre draws in a worker, served from this site. Its default address is
// relative to the library, which the bundler moves.
setWorkerUrl(workerUrl);

// The base map: OpenFreeMap's Liberty style (docs/data-sources.md).
const baseMapStyle = "https://tiles.openfreemap.org/styles/liberty";

// Switzerland, with a margin for the stations near the border.
const switzerland: [[number, number], [number, number]] = [
  [5.8, 45.75],
  [10.6, 47.85],
];

// Stations are grouped up to this zoom; beyond it, each one shows.
const clusterMaxZoom = 8;

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

type Status = "loading" | "ready" | "error";

export default function StationMap({ locale, apiUrl }: Props) {
  const container = useRef<HTMLElement>(null);
  const map = useRef<MapLibreMap | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [selected, setSelected] = useState<string | null>(null);
  const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);

  useEffect(() => {
    if (container.current === null) {
      return;
    }
    const api = createApiClient(apiUrl, browserTimeoutMs);
    const instance = new MapLibreMap({
      container: container.current,
      style: baseMapStyle,
      bounds: switzerland,
      maxBounds: [
        [3.5, 44.5],
        [13, 49],
      ],
      attributionControl: false,
      dragRotate: false,
      pitchWithRotate: false,
    });
    map.current = instance;
    instance.touchZoomRotate.disableRotation();
    instance.addControl(
      new AttributionControl({
        compact: true,
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

    let cancelled = false;
    instance.on("load", async () => {
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
        filter: ["==", ["get", "id"], ""],
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
        const { data, response } = await api.GET("/v1/stations");
        if (data === undefined) {
          throw new Error(`The API answered ${response.status}`);
        }
        if (!cancelled) {
          instance
            .getSource<GeoJSONSource>("stations")
            ?.setData(toMapData(data));
          setStatus("ready");
        }
      } catch (cause) {
        console.error("Loading the stations failed:", cause);
        if (!cancelled) {
          setStatus("error");
        }
      }
    });

    instance.on("click", "clusters", async (event) => {
      const feature = event.features?.[0];
      const clusterId = feature?.properties?.cluster_id;
      if (feature?.geometry.type !== "Point" || typeof clusterId !== "number") {
        return;
      }
      const zoom = await instance
        .getSource<GeoJSONSource>("stations")
        ?.getClusterExpansionZoom(clusterId);
      instance.easeTo({
        center: feature.geometry.coordinates as [number, number],
        zoom: Math.min(zoom ?? instance.getZoom() + 2, clusterMaxZoom + 1),
      });
    });
    instance.on("click", "stations", (event) => {
      const id = event.features?.[0]?.properties?.id;
      if (typeof id === "string") {
        setSelected(id);
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

    return () => {
      cancelled = true;
      map.current = null;
      instance.remove();
    };
  }, [apiUrl, locale]);

  // Outline the station whose sheet is open.
  useEffect(() => {
    const instance = map.current;
    if (instance?.getLayer("selected")) {
      instance.setFilter("selected", ["==", ["get", "id"], selected ?? ""]);
    }
  }, [selected]);

  return (
    <div className="station-map">
      <section
        ref={container}
        className="station-map__canvas"
        aria-label={t("map.label")}
      />
      {status !== "ready" && (
        <p
          className={`station-map__status station-map__status--${status}`}
          role="status"
        >
          {status === "loading" ? t("map.loading") : t("map.error")}
        </p>
      )}
      {selected !== null && (
        <StationSheet
          key={selected}
          id={selected}
          locale={locale}
          apiUrl={apiUrl}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}
