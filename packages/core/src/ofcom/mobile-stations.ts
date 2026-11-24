/**
 * Parser for OFCOM's public dataset of mobile phone base stations
 * (`ch.bakom.standorte-mobilfunkanlagen`). The file format and its vocabulary
 * are described in docs/data-sources.md.
 *
 * The file is a GeoJSON FeatureCollection in LV95 whose properties are display
 * strings repeated in four languages. This module reads the English ones and
 * turns them into a typed, language-neutral model.
 */

/** A position in the Swiss LV95 grid (EPSG:2056), in metres. */
export interface Lv95 {
  east: number;
  north: number;
}

export type Operator = "swisscom" | "sunrise" | "salt" | "sbb" | "other";

export type Technology = "2G" | "3G" | "4G" | "5G";

export type StationKind =
  | "outdoor"
  | "outdoor_low_power"
  | "indoor"
  | "tunnel"
  | "temporary"
  | "unknown";

/** Class of ERP90, the highest cumulated power in a 90° sector. */
export type PowerClass = "very_low" | "low" | "medium" | "high" | "unknown";

export type Permit =
  /** A site data sheet was filed; the date is the one OFCOM publishes. */
  | { type: "site_data_sheet"; date: string }
  /** Exempt from the precautionary emission limit (NISV Annex 1 No. 64). */
  | { type: "exempt" };

export interface MobileStation {
  /** The station label exactly as published, e.g. "Sunrise ZH601-1". */
  name: string;
  operator: Operator;
  /** The label without the operator prefix, e.g. "ZH601-1". */
  code: string;
  kind: StationKind;
  powerClass: PowerClass;
  technologies: Technology[];
  /**
   * True when at least part of the site runs adaptive antennas. Null when the
   * file says something about it that this parser does not recognise.
   */
  adaptive: boolean | null;
  /** Null when OFCOM publishes no permit information for the site. */
  permit: Permit | null;
  /** Installation limit value in V/m. Null when absent or left blank. */
  installationLimitValue: number | null;
  /**
   * Position of the site's first mast, as published. The database converts it
   * to WGS84: PostGIS applies the rigorous transformation, which the usual
   * approximate formulas miss by up to three metres near Geneva.
   */
  lv95: Lv95;
}

export interface ParsedMobileStations {
  stations: MobileStation[];
  /**
   * Values this parser did not recognise. The station is still returned, with
   * the field set to "unknown" or null, so one new upstream label never blocks
   * an import. Callers decide whether warnings should fail their run.
   */
  warnings: string[];
}

/** The input is not the dataset this parser understands. Nothing is returned. */
export class OfcomFormatError extends Error {
  override name = "OfcomFormatError";
}

const EXPECTED_CRS = "EPSG:2056";

const OPERATORS: Record<string, Operator> = {
  Swisscom: "swisscom",
  Sunrise: "sunrise",
  Salt: "salt",
  SBB: "sbb",
};

const KINDS: Record<string, StationKind> = {
  "Outdoor > 6 Werp": "outdoor",
  "Outdoor ≤ 6 Werp": "outdoor_low_power",
  "Indoor ≤ 6 Werp": "indoor",
  Tunnel: "tunnel",
  "Temporary station": "temporary",
  // About a hundred sites are published without a type.
  "": "unknown",
};

const POWER_CLASSES: Record<string, PowerClass> = {
  "very low": "very_low",
  low: "low",
  medium: "medium",
  high: "high",
};

const TECHNOLOGIES: readonly Technology[] = ["2G", "3G", "4G", "5G"];

const ADAPTIVE: Record<string, boolean> = {
  "": false,
  "Partially adaptive operation": true,
};
const EXEMPT_MARKER = "NISV Annex 1 No. 64";

/**
 * Parses a whole snapshot.
 *
 * @throws OfcomFormatError when the document is not a FeatureCollection in
 * LV95 or a feature lacks the fields every station has. Coordinates read in
 * the wrong reference system would be silently wrong, so that case stops the
 * run instead of producing warnings.
 */
export function parseMobileStations(document: unknown): ParsedMobileStations {
  if (!isRecord(document) || document.type !== "FeatureCollection") {
    throw new OfcomFormatError("Expected a GeoJSON FeatureCollection.");
  }
  const crs = readCrs(document.crs);
  if (crs !== EXPECTED_CRS) {
    throw new OfcomFormatError(
      `Expected coordinates in ${EXPECTED_CRS}, found ${crs ?? "no CRS"}.`,
    );
  }
  if (!Array.isArray(document.features)) {
    throw new OfcomFormatError("The FeatureCollection has no features array.");
  }

  const stations: MobileStation[] = [];
  const warnings: string[] = [];
  document.features.forEach((feature, index) => {
    stations.push(parseFeature(feature, index, warnings));
  });
  return { stations, warnings };
}

function parseFeature(
  feature: unknown,
  index: number,
  warnings: string[],
): MobileStation {
  const where = `Feature ${index}`;
  if (!isRecord(feature) || !isRecord(feature.properties)) {
    throw new OfcomFormatError(`${where} has no properties.`);
  }
  const properties = feature.properties;
  const name = readString(properties, "station", where);
  if (name === "") {
    throw new OfcomFormatError(`${where} has an empty station label.`);
  }
  const lv95 = readPosition(feature.geometry, `${where} (${name})`);
  const warn = (field: string, value: string) =>
    warnings.push(`${name}: unrecognised ${field} "${value}"`);

  const kindLabel = readString(properties, "typ_en", where);
  const kind = lookup(KINDS, kindLabel);
  if (kind === undefined) warn("type", kindLabel);

  const powerLabel = readString(properties, "power_en", where);
  const powerMatch = /^Power class : (very low|low|medium|high) \(/.exec(
    powerLabel,
  );
  const powerClass = lookup(POWER_CLASSES, powerMatch?.[1] ?? "");
  if (powerClass === undefined) warn("power class", powerLabel);

  const technologyLabel = readString(properties, "techno_en", where);
  // Published as "Technology 3G,4G,5G". Only that exact shape is read: the
  // prefix, then known items separated by commas. Anything else, such as an
  // unknown item beside known ones or another separator, adds a warning.
  const listed =
    /^Technology (.+)$/
      .exec(technologyLabel)?.[1]
      ?.split(",")
      .map((item) => item.trim()) ?? [];
  const technologies = TECHNOLOGIES.filter((technology) =>
    listed.includes(technology),
  );
  if (listed.length === 0 || technologies.length !== listed.length) {
    warn("technology", technologyLabel);
  }

  const adaptiveLabel = readString(properties, "adaptiv_en", where);
  const adaptive = lookup(ADAPTIVE, adaptiveLabel);
  if (adaptive === undefined) warn("adaptive operation", adaptiveLabel);

  return {
    name,
    ...splitName(name),
    kind: kind ?? "unknown",
    powerClass: powerClass ?? "unknown",
    technologies,
    adaptive: adaptive ?? null,
    permit: parsePermit(
      readString(properties, "bewilligung_en", where),
      (value) => warn("permit", value),
    ),
    installationLimitValue: parseLimitValue(
      readString(properties, "agw_en", where),
      (value) => warn("installation limit value", value),
    ),
    lv95,
  };
}

/**
 * The three operators and SBB are published as "Operator CODE". Foreign
 * stations near the border come as a bare code such as "DTM-SCHL" and are
 * grouped under "other" with the whole label as their code.
 */
function splitName(name: string): { operator: Operator; code: string } {
  const space = name.indexOf(" ");
  const operator =
    space > 0 ? lookup(OPERATORS, name.slice(0, space)) : undefined;
  if (operator === undefined) return { operator: "other", code: name };
  return { operator, code: name.slice(space + 1) };
}

function parsePermit(
  label: string,
  warn: (value: string) => void,
): Permit | null {
  if (label === "") return null;
  const dataSheet = /^Site data sheet (\d{4}-\d{2}-\d{2})$/.exec(label);
  if (dataSheet?.[1] !== undefined) {
    return { type: "site_data_sheet", date: dataSheet[1] };
  }
  if (label.includes(EXEMPT_MARKER)) return { type: "exempt" };
  warn(label);
  return null;
}

function parseLimitValue(
  label: string,
  warn: (value: string) => void,
): number | null {
  // Blank, or "- V/m" when OFCOM lists the field without a value.
  if (label === "" || label === "Installation limit value - V/m") return null;
  const match = /^Installation limit value (\d+(?:\.\d+)?) V\/m$/.exec(label);
  if (match?.[1] === undefined) {
    warn(label);
    return null;
  }
  return Number(match[1]);
}

/**
 * Reads a label from one of the tables above. A plain `table[label]` would
 * also find inherited properties: the label "constructor" would come back as
 * a function instead of as unknown.
 */
function lookup<T>(table: Record<string, T>, label: string): T | undefined {
  return Object.hasOwn(table, label) ? table[label] : undefined;
}

function readPosition(geometry: unknown, where: string): Lv95 {
  if (
    isRecord(geometry) &&
    geometry.type === "Point" &&
    Array.isArray(geometry.coordinates)
  ) {
    const [east, north] = geometry.coordinates;
    if (Number.isFinite(east) && Number.isFinite(north)) {
      return { east: east as number, north: north as number };
    }
  }
  throw new OfcomFormatError(`${where} has no point geometry.`);
}

function readCrs(crs: unknown): string | undefined {
  if (!isRecord(crs) || !isRecord(crs.properties)) return undefined;
  const name = crs.properties.name;
  return typeof name === "string" ? name : undefined;
}

function readString(
  properties: Record<string, unknown>,
  key: string,
  where: string,
): string {
  const value = properties[key];
  if (typeof value !== "string") {
    throw new OfcomFormatError(`${where} has no "${key}" property.`);
  }
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
