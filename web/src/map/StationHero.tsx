import { AttributionControl, Map as MapLibreMap, Marker } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useEffect, useRef } from "react";
import type { Locale } from "../i18n/locales";
import { loadBaseMap } from "./basemap";
import { localizeLabel, type Operator, operatorColors } from "./stations";
import "./map.css";

interface Props {
  locale: Locale;
  operator: Operator;
  position: { lat: number; lon: number };
}

// The map at the head of a station's page: the station where it stands, on
// the base map, to look at. The map of every station is one link away.
export default function StationHero({ locale, operator, position }: Props) {
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = container.current;
    if (element === null) {
      return;
    }
    let cancelled = false;
    let instance: MapLibreMap | undefined;
    const center: [number, number] = [position.lon, position.lat];

    loadBaseMap().then(({ style }) => {
      if (cancelled) {
        return;
      }
      try {
        instance = new MapLibreMap({
          container: element,
          style,
          center,
          zoom: 16,
          interactive: false,
          attributionControl: false,
        });
      } catch (cause) {
        // Most often a browser without WebGL: the page keeps its background.
        console.error("Creating the map failed:", cause);
        return;
      }
      // The card of the page covers the bottom of the map, and on a phone
      // the header, on two rows, its top: the station is drawn between them.
      const phone = window.matchMedia("(max-width: 640px)").matches;
      instance.setPadding({
        top: phone ? 110 : 0,
        right: 0,
        bottom: phone ? 128 : 140,
        left: 0,
      });
      // At the top right, under the pages and languages: at the bottom, the
      // card of the page would cover it.
      instance.addControl(
        new AttributionControl({ compact: true }),
        "top-right",
      );
      // On a phone the credits open folded, as on the map of every station:
      // unfolded, they would cover the station. MapLibre unfolds them the
      // first time they fill, with the style; they fold right after. Their
      // button unfolds them.
      if (phone) {
        const fold = () => {
          const credits = element.querySelector(
            ".maplibregl-ctrl-attrib.maplibregl-compact",
          );
          if (credits) {
            credits.classList.remove("maplibregl-compact-show");
            instance?.off("styledata", fold);
          }
        };
        instance.on("styledata", fold);
      }
      // The station is a marker, drawn whether or not the base map loads.
      const marker = document.createElement("div");
      marker.className = "station-hero-marker";
      marker.style.setProperty("--operator", operatorColors[operator]);
      new Marker({ element: marker }).setLngLat(center).addTo(instance);
      instance.on("load", () => {
        if (!instance) {
          return;
        }
        for (const layer of instance.getStyle().layers) {
          if (layer.type === "symbol" && layer.layout?.["text-field"]) {
            instance.setLayoutProperty(
              layer.id,
              "text-field",
              localizeLabel(layer.layout["text-field"], locale),
            );
          }
        }
      });
    });

    return () => {
      cancelled = true;
      instance?.remove();
    };
  }, [locale, operator, position.lat, position.lon]);

  return <div ref={container} className="station-hero absolute! inset-0" />;
}
