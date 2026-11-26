import type { components } from "../api/schema";
import type { Locale } from "../i18n/locales";

export type StationCollection = components["schemas"]["StationCollection"];
export type StationHistory = components["schemas"]["StationHistory"];
export type Period = components["schemas"]["Period"];
export type Operator = components["schemas"]["StationProperties"]["operator"];

// The classes shared by the sheet and the list of stations at one place.
export const mutedText = "text-[0.8125rem] text-muted";
export const operatorDot = "size-2.5 flex-none rounded-full";

// One colour per operator, readable on the base map and with white text or a
// white outline.
export const operatorColors: Record<Operator, string> = {
  swisscom: "#2563eb",
  sunrise: "#e11d48",
  salt: "#0d9488",
  sbb: "#d97706",
  other: "#7c3aed",
};

export type Kind = components["schemas"]["StationProperties"]["kind"];
export type PowerClass =
  components["schemas"]["StationProperties"]["powerClass"];
export type Technology =
  components["schemas"]["StationProperties"]["technologies"][number];

// What the map keeps of a station: its identifier, to open its sheet, its
// label, to list it among others at the same place, and its operator, to
// colour it.
export interface MapStation {
  id: string;
  name: string;
  operator: Operator;
}

// A station as the browser holds it: what the map draws, what the filters
// read, and where it stands.
export interface Station extends MapStation {
  kind: Kind;
  powerClass: PowerClass;
  technologies: Technology[];
  coordinates: [number, number];
}

export function toStations(collection: StationCollection): Station[] {
  return collection.features.map((feature) => ({
    id: feature.id,
    name: feature.properties.name,
    operator: feature.properties.operator,
    kind: feature.properties.kind,
    powerClass: feature.properties.powerClass,
    technologies: feature.properties.technologies,
    coordinates: [
      feature.geometry.coordinates[0] ?? 0,
      feature.geometry.coordinates[1] ?? 0,
    ],
  }));
}

// The stations as the map draws them. The identifier is copied into the
// properties, where MapLibre's expressions can read it.
export function toMapData(stations: Station[]) {
  return {
    type: "FeatureCollection" as const,
    features: stations.map((station) => ({
      type: "Feature" as const,
      geometry: { type: "Point" as const, coordinates: station.coordinates },
      properties: {
        id: station.id,
        name: station.name,
        operator: station.operator,
      } satisfies MapStation,
    })),
  };
}

// Whether a value has the shape of a station's identifier, `east_north_label`
// (decision 22), as the API's contract describes it: what an address names
// is checked before it becomes a path.
export function isStationId(value: string): boolean {
  return (
    // In characters, as the contract counts them, not in UTF-16 units.
    [...value].length <= 200 &&
    /^-?\d+(?:\.\d+)?_-?\d+(?:\.\d+)?_[^\r\n]+$/.test(value)
  );
}

// The LV95 position in a station's identifier, `east_north_label`
// (decision 22), to tell apart stations that share a label.
export function lv95Of(id: string): string {
  const [east = "", north = ""] = id.split("_");
  return `LV95 ${east}, ${north}`;
}

// The distinct stations among the features under a click. Several stations
// can stand at the same position: 1,417 places held more than one in the
// snapshot of 24 November 2026.
export function stationsAt(
  features: { properties?: Record<string, unknown> | null }[],
): MapStation[] {
  const found = new Map<string, MapStation>();
  for (const { properties } of features) {
    const id = properties?.id;
    const name = properties?.name;
    const operator = properties?.operator;
    if (typeof id === "string" && typeof name === "string" && !found.has(id)) {
      // An operator the contract adds later is drawn as `other`, and listed so.
      const known =
        typeof operator === "string" && Object.hasOwn(operatorColors, operator)
          ? (operator as Operator)
          : "other";
      found.set(id, { id, name, operator: known });
    }
  }
  return [...found.values()];
}

// The base map writes place names in English where OpenStreetMap has an
// English name. This rewrites a label expression to take the name in the
// visitor's language, then the local one: Genève, Genf, Ginevra.
export function localizeLabel<T>(expression: T, locale: Locale): T {
  if (!Array.isArray(expression)) {
    return expression;
  }
  if (
    locale !== "en" &&
    expression.length === 2 &&
    expression[0] === "get" &&
    expression[1] === "name_en"
  ) {
    return ["get", `name:${locale}`] as T;
  }
  return expression.map((item: unknown) => localizeLabel(item, locale)) as T;
}
