import { type StyleSpecification, setWorkerUrl } from "maplibre-gl";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?url";
import { browserTimeoutMs } from "../api/client";

// What the map of every station and the map at the head of a station's page
// share: MapLibre's worker, the base map and what replaces it when it fails.

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

// The base map's style, read here rather than by MapLibre, which would never
// finish loading without it: when it fails, the map is drawn on a plain
// background, and `failed` says so.
export async function loadBaseMap(): Promise<{
  style: StyleSpecification;
  failed: boolean;
}> {
  try {
    const response = await fetch(baseMapStyle, {
      signal: AbortSignal.timeout(browserTimeoutMs),
    });
    if (!response.ok) {
      throw new Error(`The base map answered ${response.status}`);
    }
    return {
      style: (await response.json()) as StyleSpecification,
      failed: false,
    };
  } catch (cause) {
    console.error("Loading the base map failed:", cause);
    return { style: fallbackStyle, failed: true };
  }
}
