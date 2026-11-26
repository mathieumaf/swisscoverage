import { type ReactNode, useId, useMemo, useState } from "react";
import { formatNumber, type Locale } from "../i18n/locales";
import type { MessageKey } from "../i18n/messages/en";
import { translate } from "../i18n/translate";
import {
  allFilters,
  type Filters,
  isEveryStationShown,
  kinds,
  operators,
  powerClasses,
  searchStations,
  technologies,
} from "./filters";
import {
  lv95Of,
  mutedText,
  operatorColors,
  operatorDot,
  type Station,
} from "./stations";

interface Props {
  locale: Locale;
  filters: Filters;
  onFilters: (filters: Filters) => void;
  // Every station, for the search, and how many the filters keep.
  stations: Station[];
  shown: number;
  onFind: (station: Station) => void;
  open: boolean;
  onOpen: (open: boolean) => void;
}

// How many stations the search lists at most.
const searchLimit = 8;

// The panel of filters and the search by label. On a large screen it stands
// at the top right of the map; on a phone it folds into a button and opens
// from the bottom.
export function FilterPanel({
  locale,
  filters,
  onFilters,
  stations,
  shown,
  onFind,
  open,
  onOpen,
}: Props) {
  const t = (key: MessageKey, values?: Record<string, string>) =>
    translate(locale, key, values);
  const [query, setQuery] = useState("");
  const found = useMemo(
    () => searchStations(stations, query, searchLimit),
    [stations, query],
  );
  const searchId = useId();
  const count = t("filters.count", {
    shown: formatNumber(locale, shown),
    total: formatNumber(locale, stations.length),
  });

  function toggle<K extends keyof Filters>(
    key: K,
    value: Filters[K] extends Set<infer V> ? V : never,
  ) {
    const next = new Set(filters[key] as Set<typeof value>);
    if (next.has(value)) {
      next.delete(value);
    } else {
      next.add(value);
    }
    onFilters({ ...filters, [key]: next });
  }

  if (!open) {
    return (
      <button
        type="button"
        className="absolute top-3 right-12 z-[1] cursor-pointer rounded-full border border-border bg-background px-4 py-2 text-[0.875rem] font-semibold text-foreground shadow-[0_2px_8px_rgb(0_0_0/0.2)] phone:top-auto phone:right-auto phone:bottom-10 phone:left-3"
        aria-expanded="false"
        onClick={() => onOpen(true)}
      >
        {t("filters.title")}
        {!isEveryStationShown(filters) && (
          <span className="ml-2 font-normal text-muted">{count}</span>
        )}
      </button>
    );
  }

  return (
    <section
      className="absolute top-3 right-12 z-[1] max-h-[calc(100%-4rem)] w-80 overflow-y-auto rounded-xl border border-border bg-background px-4 py-3 text-[0.875rem] text-foreground shadow-[0_8px_24px_rgb(0_0_0/0.2)] phone:top-auto phone:right-0 phone:bottom-10 phone:left-0 phone:max-h-[65%] phone:w-auto phone:rounded-b-none"
      aria-label={t("filters.title")}
    >
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-[1rem] font-bold">{t("filters.title")}</h2>
          <p className={mutedText} role="status">
            {count}
          </p>
        </div>
        <button
          type="button"
          className="size-9 flex-none cursor-pointer rounded-full border border-border text-[1.25rem] leading-none"
          aria-expanded="true"
          aria-label={t("filters.close")}
          onClick={() => onOpen(false)}
        >
          ×
        </button>
      </header>

      <div className="mt-3">
        <label htmlFor={searchId} className="font-semibold">
          {t("search.label")}
        </label>
        <input
          id={searchId}
          type="search"
          className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-foreground"
          placeholder={t("search.placeholder")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          autoComplete="off"
          spellCheck={false}
        />
        {query.trim() !== "" &&
          (found.length === 0 ? (
            <p className={`mt-2 ${mutedText}`}>{t("search.none")}</p>
          ) : (
            <ul className="mt-1">
              {found.map((station) => (
                <li key={station.id}>
                  <button
                    type="button"
                    className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-border focus-visible:bg-border"
                    onClick={() => onFind(station)}
                  >
                    <span
                      className={operatorDot}
                      style={{
                        background:
                          operatorColors[station.operator] ??
                          operatorColors.other,
                      }}
                    />
                    {/* One label can stand at several positions. */}
                    <span>
                      {station.name}
                      <br />
                      <span className={mutedText}>{lv95Of(station.id)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          ))}
      </div>

      <Group title={t("filters.operator")}>
        {operators.map((operator) => (
          <Chip
            key={operator}
            pressed={filters.operators.has(operator)}
            onClick={() => toggle("operators", operator)}
          >
            <span
              className={operatorDot}
              style={{ background: operatorColors[operator] }}
            />
            {t(`operator.${operator}`)}
          </Chip>
        ))}
      </Group>
      <Group title={t("filters.technology")}>
        {technologies.map((technology) => (
          <Chip
            key={technology}
            pressed={filters.technologies.has(technology)}
            onClick={() => toggle("technologies", technology)}
          >
            {technology}
          </Chip>
        ))}
      </Group>
      <Group title={t("filters.kind")}>
        {kinds.map((kind) => (
          <Chip
            key={kind}
            pressed={filters.kinds.has(kind)}
            onClick={() => toggle("kinds", kind)}
          >
            {t(`kind.${kind}`)}
          </Chip>
        ))}
      </Group>
      <Group title={t("filters.powerClass")}>
        {powerClasses.map((powerClass) => (
          <Chip
            key={powerClass}
            pressed={filters.powerClasses.has(powerClass)}
            onClick={() => toggle("powerClasses", powerClass)}
          >
            {t(`power.${powerClass}`)}
          </Chip>
        ))}
      </Group>

      {!isEveryStationShown(filters) && (
        <button
          type="button"
          className="mt-4 cursor-pointer text-accent underline"
          onClick={() => onFilters(allFilters())}
        >
          {t("filters.reset")}
        </button>
      )}
    </section>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="mt-4">
      <legend className="mb-1.5 font-semibold">{title}</legend>
      <div className="flex flex-wrap gap-1.5">{children}</div>
    </fieldset>
  );
}

// A value of a vocabulary, kept when pressed.
function Chip({
  pressed,
  onClick,
  children,
}: {
  pressed: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      onClick={onClick}
      className={`flex cursor-pointer items-center gap-1.5 rounded-md border px-2 py-1 text-[0.8125rem] font-semibold ${
        pressed
          ? "border-accent bg-accent text-background"
          : "border-border bg-background text-muted line-through decoration-1"
      }`}
    >
      {children}
    </button>
  );
}
