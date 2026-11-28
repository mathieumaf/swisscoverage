import { formatDate, formatNumber, type Locale } from "../i18n/locales";
import type { MessageKey } from "../i18n/messages/en";
import { translator } from "../i18n/translate";
import type { Period, StationHistory } from "./stations";

// What a period says, in words, for the sheet on the map and the station's
// page.
export interface PeriodText {
  kind: string;
  powerClass: string;
  technologies: string;
  adaptive: string;
  limitValue: string;
  permit: string;
}

export const periodFields: { key: keyof PeriodText; label: MessageKey }[] = [
  { key: "kind", label: "sheet.kind" },
  { key: "powerClass", label: "sheet.powerClass" },
  { key: "technologies", label: "sheet.technologies" },
  { key: "adaptive", label: "sheet.adaptive" },
  { key: "limitValue", label: "sheet.limitValue" },
  { key: "permit", label: "sheet.permit" },
];

// Where a station stands: in WGS84, to five decimals, about a metre, and in
// LV95 as OFCOM publishes it.
export function positionText(
  station: Pick<StationHistory, "position" | "lv95">,
): { wgs84: string; lv95: string } {
  const { position, lv95 } = station;
  return {
    wgs84: `${position.lat.toFixed(5)}, ${position.lon.toFixed(5)}`,
    lv95: `LV95 ${lv95.east}, ${lv95.north}`,
  };
}

export function describePeriod(locale: Locale, period: Period): PeriodText {
  const t = translator(locale);
  return {
    kind: t(`kind.${period.kind}`),
    powerClass: t(`power.${period.powerClass}`),
    technologies:
      period.technologies.length === 0
        ? t("sheet.noTechnology")
        : period.technologies.join(", "),
    adaptive:
      period.adaptive === null
        ? t("sheet.unknown")
        : t(period.adaptive ? "adaptive.yes" : "adaptive.no"),
    limitValue:
      period.installationLimitValue === null
        ? t("sheet.unknown")
        : `${formatNumber(locale, period.installationLimitValue, {
            minimumFractionDigits: 1,
            maximumFractionDigits: 4,
          })} V/m`,
    permit: permitText(locale, period),
  };
}

function permitText(locale: Locale, period: Period): string {
  const t = translator(locale);
  if (period.permit === null) {
    return t("sheet.unknown");
  }
  if (period.permit.type === "exempt") {
    return t("permit.exempt");
  }
  return period.permit.date === undefined
    ? t("permit.site_data_sheet")
    : t("permit.site_data_sheet_dated", {
        date: formatDate(locale, new Date(period.permit.date)),
      });
}

export interface Change {
  label: MessageKey;
  before: string;
  after: string;
}

// One line of a station's history, the latest first: a period with what
// changed from the one before it, or a time when the site was not published.
export type HistoryEntry =
  | {
      type: "period";
      from: string;
      to: string | null;
      // Empty for the oldest period shown.
      changes: Change[];
      // The first period the archive holds for the site.
      first: boolean;
    }
  | { type: "gap"; from: string; to: string };

// The history of a station from its periods, the latest first, as the API
// gives them. Each period lists only what differs from the one before it, so
// that a change of permit does not repeat every other field. `complete` says
// whether the oldest of `periods` is the site's first.
export function history(
  locale: Locale,
  periods: Period[],
  complete: boolean,
): HistoryEntry[] {
  const entries: HistoryEntry[] = [];
  for (const [index, period] of periods.entries()) {
    const older = periods[index + 1];
    entries.push({
      type: "period",
      from: period.from,
      to: period.to,
      changes: older === undefined ? [] : changes(locale, older, period),
      first: older === undefined && complete,
    });
    if (older?.to != null && older.to !== period.from) {
      entries.push({ type: "gap", from: older.to, to: period.from });
    }
  }
  return entries;
}

// What each field holds, as the API gives it, so that a change shows even
// when its words round to the same.
function value(period: Period, key: keyof PeriodText): string {
  switch (key) {
    case "kind":
      return period.kind;
    case "powerClass":
      return period.powerClass;
    case "technologies":
      return period.technologies.join(",");
    case "adaptive":
      return String(period.adaptive);
    case "limitValue":
      return String(period.installationLimitValue);
    case "permit":
      return JSON.stringify(period.permit);
  }
}

function changes(locale: Locale, older: Period, newer: Period): Change[] {
  const before = describePeriod(locale, older);
  const after = describePeriod(locale, newer);
  return periodFields
    .filter(({ key }) => value(older, key) !== value(newer, key))
    .map(({ key, label }) => ({
      label,
      before: before[key],
      after: after[key],
    }));
}
