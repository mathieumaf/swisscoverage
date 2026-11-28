import { type ReactNode, useId, useMemo, useState } from "react";
import { formatNumber, type Locale } from "../i18n/locales";
import { translator } from "../i18n/translate";
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
import { type Place, placeAttribution } from "./places";
import {
  lv95Of,
  mutedText,
  operatorColors,
  operatorDot,
  type Station,
} from "./stations";
import { usePlaces } from "./usePlaces";

interface Props {
  locale: Locale;
  filters: Filters;
  onFilters: (filters: Filters) => void;
  // Every station, for the search, and how many the filters keep.
  stations: Station[];
  shown: number;
  onFind: (station: Station) => void;
  onPlace: (place: Place) => void;
  // The search was emptied: the place it found is no longer marked.
  onClearPlace: () => void;
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
  onPlace,
  onClearPlace,
  open,
  onOpen,
}: Props) {
  const t = translator(locale);
  const [query, setQuery] = useState("");
  const found = useMemo(
    () => searchStations(stations, query, searchLimit),
    [stations, query],
  );
  const placeSearch = usePlaces(query);
  // A guess of the service is noise next to the stations a label matches.
  const places =
    placeSearch.status === "ready" && !(placeSearch.fuzzy && found.length > 0)
      ? placeSearch.places
      : [];
  const searched = query.trim() !== "";
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
        className="absolute top-3 right-12 z-[1] cursor-pointer rounded-full border border-border bg-background px-4 py-2 text-[0.875rem] font-semibold text-foreground shadow-[0_2px_8px_rgb(0_0_0/0.2)] phone:top-auto phone:right-auto phone:bottom-24 phone:left-3"
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

      <search className="mt-3">
        <form
          onSubmit={(event) => {
            // Enter chooses the first result.
            event.preventDefault();
            const [station] = found;
            const [place] = places;
            if (station) {
              onFind(station);
            } else if (place) {
              onPlace(place);
            }
          }}
        >
          <label htmlFor={searchId} className="font-semibold">
            {t("search.label")}
          </label>
          <input
            id={searchId}
            type="search"
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-foreground"
            placeholder={t("search.placeholder")}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              if (event.target.value.trim() === "") {
                onClearPlace();
              }
            }}
            autoComplete="off"
            spellCheck={false}
          />
          {searched && found.length > 0 && (
            <Results title={t("search.stations")}>
              {found.map((station) => (
                <li key={station.id}>
                  <button
                    type="button"
                    className={resultButton}
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
            </Results>
          )}
          {places.length > 0 && (
            <Results title={t("search.places")}>
              {places.map((place) => (
                <li key={place.key}>
                  <button
                    type="button"
                    className={resultButton}
                    onClick={() => onPlace(place)}
                  >
                    <span className="size-2.5 flex-none rounded-full border-2 border-accent" />
                    <span>
                      {place.label}
                      <br />
                      <span className={mutedText}>
                        {t(`place.${place.origin}`)}
                      </span>
                    </span>
                  </button>
                </li>
              ))}
              <li className={`px-2 pt-1 ${mutedText}`}>{placeAttribution}</li>
            </Results>
          )}
          {searched && placeSearch.status === "loading" && (
            <p className={`mt-2 ${mutedText}`} role="status">
              {t("search.placesLoading")}
            </p>
          )}
          {searched && placeSearch.status === "error" && (
            <p className={`mt-2 ${mutedText}`} role="status">
              {t("search.placesError")}
            </p>
          )}
          {searched &&
            found.length === 0 &&
            (placeSearch.status === "idle" ||
              (placeSearch.status === "ready" && places.length === 0)) && (
              <p className={`mt-2 ${mutedText}`} role="status">
                {placeSearch.status === "idle"
                  ? t("search.none")
                  : t("search.nothing")}
              </p>
            )}
        </form>
      </search>

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

const resultButton =
  "flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-border focus-visible:bg-border";

function Results({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-2">
      <h3 className={`px-2 ${mutedText}`}>{title}</h3>
      <ul>{children}</ul>
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
