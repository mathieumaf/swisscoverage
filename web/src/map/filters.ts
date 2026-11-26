import type {
  Kind,
  Operator,
  PowerClass,
  Station,
  Technology,
} from "./stations";

// Every value of each vocabulary, in the order the panel shows them. The
// records make the type checker fail when the contract adds a value.
const operatorOrder: Record<Operator, number> = {
  swisscom: 0,
  sunrise: 1,
  salt: 2,
  sbb: 3,
  other: 4,
};
const technologyOrder: Record<Technology, number> = {
  "2G": 0,
  "3G": 1,
  "4G": 2,
  "5G": 3,
};
const kindOrder: Record<Kind, number> = {
  outdoor: 0,
  outdoor_low_power: 1,
  indoor: 2,
  tunnel: 3,
  temporary: 4,
  unknown: 5,
};
const powerClassOrder: Record<PowerClass, number> = {
  very_low: 0,
  low: 1,
  medium: 2,
  high: 3,
  unknown: 4,
};

function ordered<T extends string>(order: Record<T, number>): T[] {
  return (Object.keys(order) as T[]).sort((a, b) => order[a] - order[b]);
}

export const operators = ordered(operatorOrder);
export const technologies = ordered(technologyOrder);
export const kinds = ordered(kindOrder);
export const powerClasses = ordered(powerClassOrder);

// What the visitor chose to see: the values kept in each vocabulary. A
// station shows when its operator, its kind and its power class are kept,
// and at least one of its technologies is, unless every technology is kept.
export interface Filters {
  operators: Set<Operator>;
  technologies: Set<Technology>;
  kinds: Set<Kind>;
  powerClasses: Set<PowerClass>;
}

export function allFilters(): Filters {
  return {
    operators: new Set(operators),
    technologies: new Set(technologies),
    kinds: new Set(kinds),
    powerClasses: new Set(powerClasses),
  };
}

export function isEveryStationShown(filters: Filters): boolean {
  return (
    filters.operators.size === operators.length &&
    filters.technologies.size === technologies.length &&
    filters.kinds.size === kinds.length &&
    filters.powerClasses.size === powerClasses.length
  );
}

export function matches(station: Station, filters: Filters): boolean {
  return (
    filters.operators.has(station.operator) &&
    filters.kinds.has(station.kind) &&
    filters.powerClasses.has(station.powerClass) &&
    (filters.technologies.size === technologies.length ||
      station.technologies.some((technology) =>
        filters.technologies.has(technology),
      ))
  );
}

// The parameters of the address that hold the filters, one per vocabulary,
// with the kept values separated by commas. A vocabulary whose every value
// is kept has no parameter.
const parameters = {
  operators: ["operator", operators],
  technologies: ["technology", technologies],
  kinds: ["kind", kinds],
  powerClasses: ["power", powerClasses],
} as const;

// The filters an address names. A value that is not in the vocabulary is
// ignored: the address comes from whoever shared it.
export function readFilters(search: URLSearchParams): Filters {
  const filters = allFilters();
  for (const [key, [name, vocabulary]] of Object.entries(parameters)) {
    const value = search.get(name);
    if (value === null) {
      continue;
    }
    const known = new Set<string>(vocabulary);
    const kept = value.split(",").filter((item) => known.has(item));
    (filters as unknown as Record<string, Set<string>>)[key] = new Set(kept);
  }
  return filters;
}

// Writes the filters into the parameters of an address, and leaves the
// other parameters as they are.
export function writeFilters(filters: Filters, search: URLSearchParams): void {
  for (const [key, [name, vocabulary]] of Object.entries(parameters)) {
    const kept = (filters as unknown as Record<string, Set<string>>)[key];
    if (kept === undefined || kept.size === vocabulary.length) {
      search.delete(name);
    } else {
      search.set(
        name,
        (vocabulary as readonly string[])
          .filter((item) => kept.has(item))
          .join(","),
      );
    }
  }
}

// A label reduced to letters and digits, without accents or case, so that
// "zh 601", "ZH601" and "Zh-601" find "Sunrise ZH601-1".
export function normalizeLabel(label: string): string {
  return label
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]/gu, "");
}

// The stations whose label contains the query, those where it comes
// earliest first, then by label.
export function searchStations(
  stations: Station[],
  query: string,
  limit: number,
): Station[] {
  const wanted = normalizeLabel(query);
  if (wanted === "") {
    return [];
  }
  const found: { station: Station; at: number }[] = [];
  for (const station of stations) {
    const at = normalizeLabel(station.name).indexOf(wanted);
    if (at !== -1) {
      found.push({ station, at });
    }
  }
  found.sort(
    (a, b) => a.at - b.at || a.station.name.localeCompare(b.station.name),
  );
  return found.slice(0, limit).map(({ station }) => station);
}
