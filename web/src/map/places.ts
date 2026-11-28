// The search by address: swisstopo's location search (docs/data-sources.md),
// which the visitor's browser calls itself.

export const placeSearchUrl =
  "https://api3.geo.admin.ch/rest/services/ech/SearchServer";

// What the site shows wherever the places found are shown.
export const placeAttribution = "© swisstopo";

// The origins the search asks for: buildings' addresses, postcodes and
// municipalities. The others are left out (docs/data-sources.md).
export const placeOrigins = ["address", "zipcode", "gg25"] as const;
export type PlaceOrigin = (typeof placeOrigins)[number];

// How many characters the visitor types before the search asks swisstopo,
// and how long it waits for them to pause.
export const placeQueryMinLength = 3;
export const placeQueryDelayMs = 300;

const placeLimit = 5;
const placeTimeoutMs = 10_000;

// The service reads at most ten words, and nobody types a longer address.
const maxWords = 10;
const maxQueryLength = 100;

// Where a place may stand: the map's own limits.
const area = { west: 3.5, south: 44.5, east: 13, north: 49 };

export interface Place {
  // Unique within one answer.
  key: string;
  origin: PlaceOrigin;
  label: string;
  center: [number, number];
  // The extent of a postcode or a municipality, west-south then east-north.
  bounds?: [[number, number], [number, number]];
}

export interface PlaceAnswer {
  places: Place[];
  // The service found nothing that matches, and guessed.
  fuzzy: boolean;
}

// The query as the service gets it.
export function placeQuery(query: string): string {
  return query
    .trim()
    .split(/\s+/u)
    .slice(0, maxWords)
    .join(" ")
    .slice(0, maxQueryLength);
}

export function placeSearchAddress(query: string): URL {
  const url = new URL(placeSearchUrl);
  url.search = new URLSearchParams({
    type: "locations",
    origins: placeOrigins.join(","),
    sr: "4326",
    limit: String(placeLimit),
    searchText: placeQuery(query),
  }).toString();
  return url;
}

export async function searchPlaces(
  query: string,
  signal: AbortSignal,
): Promise<PlaceAnswer> {
  const response = await fetch(placeSearchAddress(query), {
    signal: AbortSignal.any([signal, AbortSignal.timeout(placeTimeoutMs)]),
    credentials: "omit",
    redirect: "error",
  });
  if (!response.ok) {
    throw new Error(`The search by address answered ${response.status}`);
  }
  return toPlaces(await response.json());
}

// What the site keeps of an answer of the service, which it does not trust:
// a result that is not of the expected shape, or outside the map, is left
// out.
export function toPlaces(answer: unknown): PlaceAnswer {
  if (!isObject(answer) || !Array.isArray(answer.results)) {
    throw new Error("The search by address answered an unexpected document");
  }
  const places: Place[] = [];
  for (const [index, result] of answer.results.entries()) {
    const place = toPlace(result, index);
    if (place !== undefined) {
      places.push(place);
    }
    if (places.length === placeLimit) {
      break;
    }
  }
  return { places, fuzzy: answer.fuzzy === "true" || answer.fuzzy === true };
}

function toPlace(result: unknown, index: number): Place | undefined {
  if (!isObject(result) || !isObject(result.attrs)) {
    return undefined;
  }
  const { origin, label, lat, lon, geom_st_box2d: box } = result.attrs;
  if (
    !isPlaceOrigin(origin) ||
    typeof label !== "string" ||
    typeof lat !== "number" ||
    typeof lon !== "number" ||
    !inArea(lon, lat)
  ) {
    return undefined;
  }
  const text = labelText(label);
  if (text === "") {
    return undefined;
  }
  const place: Place = {
    key: `${origin}:${typeof result.id === "number" ? result.id : index}:${index}`,
    origin,
    label: text,
    center: [lon, lat],
  };
  const bounds = origin === "address" ? undefined : boxBounds(box);
  if (bounds !== undefined) {
    place.bounds = bounds;
  }
  return place;
}

// A label is HTML, such as `Bundesplatz 3 <b>3011 Bern</b>`: the site keeps
// its text, and never renders it as HTML. A street without a number reads
// `Route de Lausanne #`.
export function labelText(label: string): string {
  return label
    .slice(0, 500)
    .replace(/<[^>]*>/gu, "")
    .replace(
      /&(amp|lt|gt|quot|#39|apos);/gu,
      (_, name: string) => entities[name] ?? "",
    )
    .replace(/\s#(?=\s|$)/gu, "")
    .replace(/\s+/gu, " ")
    .trim();
}

const entities: Record<string, string> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: '"',
  "#39": "'",
  apos: "'",
};

// `BOX(west south,east north)`, in WGS84 since the search asks for it.
function boxBounds(
  box: unknown,
): [[number, number], [number, number]] | undefined {
  if (typeof box !== "string") {
    return undefined;
  }
  const number = "(-?\\d+(?:\\.\\d+)?)";
  const match = new RegExp(
    `^BOX\\(${number} ${number},${number} ${number}\\)$`,
    "u",
  ).exec(box);
  if (match === null) {
    return undefined;
  }
  const [west, south, east, north] = match.slice(1).map(Number) as [
    number,
    number,
    number,
    number,
  ];
  if (
    !inArea(west, south) ||
    !inArea(east, north) ||
    west >= east ||
    south >= north
  ) {
    return undefined;
  }
  return [
    [west, south],
    [east, north],
  ];
}

function inArea(lon: number, lat: number): boolean {
  return (
    Number.isFinite(lon) &&
    Number.isFinite(lat) &&
    lon >= area.west &&
    lon <= area.east &&
    lat >= area.south &&
    lat <= area.north
  );
}

function isPlaceOrigin(value: unknown): value is PlaceOrigin {
  return (placeOrigins as readonly unknown[]).includes(value);
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
