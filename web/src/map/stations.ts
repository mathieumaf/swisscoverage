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

// What the map keeps of each station: its position, its identifier, to open
// its sheet, and its operator, to colour it. The identifier is copied into the
// properties, where MapLibre's expressions can read it.
export function toMapData(collection: StationCollection) {
  return {
    type: "FeatureCollection" as const,
    features: collection.features.map((feature) => ({
      type: "Feature" as const,
      geometry: feature.geometry,
      properties: { id: feature.id, operator: feature.properties.operator },
    })),
  };
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
