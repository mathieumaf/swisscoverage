import type { components } from "../api/schema";
import type { Locale } from "../i18n/locales";

export type StationCollection = components["schemas"]["StationCollection"];
export type StationHistory = components["schemas"]["StationHistory"];
export type Period = components["schemas"]["Period"];
export type Operator = components["schemas"]["StationProperties"]["operator"];

// One colour per operator, readable on the base map and with white text or a
// white outline.
export const operatorColors: Record<Operator, string> = {
  swisscom: "#2563eb",
  sunrise: "#e11d48",
  salt: "#0d9488",
  sbb: "#d97706",
  other: "#7c3aed",
};

// What the map keeps of a station: its identifier, to open its sheet, its
// label, to list it among others at the same place, and its operator, to
// colour it.
export interface MapStation {
  id: string;
  name: string;
  operator: Operator;
}

// The stations as the map draws them. The identifier is copied into the
// properties, where MapLibre's expressions can read it.
export function toMapData(collection: StationCollection) {
  return {
    type: "FeatureCollection" as const,
    features: collection.features.map((feature) => ({
      type: "Feature" as const,
      geometry: feature.geometry,
      properties: {
        id: feature.id,
        name: feature.properties.name,
        operator: feature.properties.operator,
      } satisfies MapStation,
    })),
  };
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
    if (
      typeof id === "string" &&
      typeof name === "string" &&
      typeof operator === "string" &&
      operator in operatorColors &&
      !found.has(id)
    ) {
      found.set(id, { id, name, operator: operator as Operator });
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
