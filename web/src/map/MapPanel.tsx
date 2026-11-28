import { type ReactNode, useEffect, useId, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import type { Locale } from "../i18n/locales";
import { translator } from "../i18n/translate";
import { searchStations } from "./filters";
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
  // Every station, for the search.
  stations: Station[];
  onFind: (station: Station) => void;
  onPlace: (place: Place) => void;
  // The search was emptied: the place it found is no longer marked.
  onClearPlace: () => void;
  // What the panel shows in place of its home: a station's sheet, or the
  // stations at one place. `detailKey` changes with it.
  detail: ReactNode;
  detailKey: string | undefined;
}

// How many stations the search lists at most.
const searchLimit = 8;

// The panel of the map, which the page renders with its home: the site's
// title, the date of the data and OFCOM's credit, shown without JavaScript.
// This fills it: the search at its top, and below it the results, a station
// or the stations at one place, in place of the home. The panel stands at the
// left of a large screen and at the bottom of a phone.
export function MapPanel({
  locale,
  stations,
  onFind,
  onPlace,
  onClearPlace,
  detail,
  detailKey,
}: Props) {
  const t = translator(locale);
  const [query, setQuery] = useState("");
  // Whether the results show: they fold once one is chosen, and come back
  // when the visitor types again.
  const [showResults, setShowResults] = useState(false);
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
  const searched = showResults && query.trim() !== "";
  const view = searched ? "search" : detailKey ? "detail" : "home";
  const searchId = useId();

  const [targets] = useState(() => ({
    panel: document.getElementById("map-panel"),
    search: document.getElementById("map-panel-search"),
    view: document.getElementById("map-panel-view"),
  }));

  // The page's home shows only while the panel has nothing else to show.
  useEffect(() => {
    if (targets.panel) {
      targets.panel.dataset.view = view;
    }
  }, [targets, view]);

  // A station opened on the map takes the place of the results.
  useEffect(() => {
    if (detailKey) {
      setShowResults(false);
    }
  }, [detailKey]);

  function choose(action: () => void) {
    setShowResults(false);
    action();
  }

  if (!targets.search || !targets.view) {
    return null;
  }

  const search = (
    <search>
      <form
        onSubmit={(event) => {
          // Enter chooses the first result.
          event.preventDefault();
          const [station] = found;
          const [place] = places;
          if (station) {
            choose(() => onFind(station));
          } else if (place) {
            choose(() => onPlace(place));
          }
        }}
      >
        <label htmlFor={searchId} className="sr-only">
          {t("search.label")}
        </label>
        <div className="flex h-12 items-center gap-2.5 rounded-2xl bg-field pr-1.5 pl-4">
          <svg
            viewBox="0 0 24 24"
            width="18"
            height="18"
            aria-hidden="true"
            className="flex-none text-muted"
          >
            <circle
              cx="11"
              cy="11"
              r="6.5"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.2"
            />
            <path
              d="M16 16l4.5 4.5"
              stroke="currentColor"
              strokeWidth="2.2"
              strokeLinecap="round"
            />
          </svg>
          <input
            id={searchId}
            type="search"
            className="h-full min-w-0 flex-1 bg-transparent text-[1rem] text-foreground outline-none placeholder:text-muted [&::-webkit-search-cancel-button]:hidden"
            placeholder={t("search.placeholder")}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setShowResults(true);
              if (event.target.value.trim() === "") {
                onClearPlace();
              }
            }}
            onFocus={() => setShowResults(true)}
            onKeyDown={(event) => {
              if (event.key === "Escape" && searched) {
                // The results fold; the open station, if any, stays.
                event.stopPropagation();
                setShowResults(false);
              }
            }}
            autoComplete="off"
            spellCheck={false}
          />
          {query !== "" && (
            <button
              type="button"
              className="grid size-9 flex-none cursor-pointer place-items-center rounded-full text-muted hover:text-foreground"
              aria-label={t("search.clear")}
              onClick={() => {
                setQuery("");
                setShowResults(false);
                onClearPlace();
              }}
            >
              <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                aria-hidden="true"
              >
                <path
                  d="M6 6l12 12M18 6L6 18"
                  stroke="currentColor"
                  strokeWidth="2.6"
                  strokeLinecap="round"
                />
              </svg>
            </button>
          )}
        </div>
      </form>
    </search>
  );

  const results = (
    // Escape on a result folds the results, as it does in the field.
    // biome-ignore lint/a11y/noStaticElementInteractions: the key comes from the buttons inside
    <div
      className="px-3 pt-2 pb-4 text-[0.9375rem]"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          setShowResults(false);
        }
      }}
    >
      {found.length > 0 && (
        <Results title={t("search.stations")}>
          {found.map((station) => (
            <li key={station.id}>
              <button
                type="button"
                className={resultButton}
                onClick={() => choose(() => onFind(station))}
              >
                <span
                  className={operatorDot}
                  style={{
                    background:
                      operatorColors[station.operator] ?? operatorColors.other,
                  }}
                />
                {/* One label can stand at several positions. */}
                <span className="min-w-0">
                  <span className="font-bold wrap-anywhere">
                    {station.name}
                  </span>
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
                onClick={() => choose(() => onPlace(place))}
              >
                <svg
                  viewBox="0 0 24 24"
                  width="18"
                  height="18"
                  aria-hidden="true"
                  className="flex-none text-accent"
                >
                  <path
                    d="M12 21s7-6.5 7-12a7 7 0 0 0-14 0c0 5.5 7 12 7 12Z"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                  />
                  <circle cx="12" cy="9" r="2.5" fill="currentColor" />
                </svg>
                <span className="min-w-0">
                  <span className="font-bold">{place.label}</span>
                  <br />
                  <span className={mutedText}>
                    {t(`place.${place.origin}`)}
                  </span>
                </span>
              </button>
            </li>
          ))}
          <li className={`px-3 pt-1 ${mutedText}`}>{placeAttribution}</li>
        </Results>
      )}
      {placeSearch.status === "loading" && (
        <p className={`mt-2 px-3 ${mutedText}`} role="status">
          {t("search.placesLoading")}
        </p>
      )}
      {placeSearch.status === "error" && (
        <p className={`mt-2 px-3 ${mutedText}`} role="status">
          {t("search.placesError")}
        </p>
      )}
      {found.length === 0 &&
        (placeSearch.status === "idle" ||
          (placeSearch.status === "ready" && places.length === 0)) && (
          <p className={`mt-2 px-3 ${mutedText}`} role="status">
            {placeSearch.status === "idle"
              ? t("search.none")
              : t("search.nothing")}
          </p>
        )}
    </div>
  );

  return (
    <>
      {createPortal(search, targets.search)}
      {/* The station stays mounted under the results: folding them shows it
          again without reading it from the API a second time. */}
      {createPortal(
        <>
          {view === "search" && results}
          <div hidden={view !== "detail"}>{detail}</div>
        </>,
        targets.view,
      )}
    </>
  );
}

const resultButton =
  "flex w-full cursor-pointer items-center gap-3 rounded-2xl px-3 py-2.5 text-left hover:bg-field focus-visible:bg-field";

function Results({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-2">
      <h3 className={`px-3 pb-1 font-bold ${mutedText}`}>{title}</h3>
      <ul>{children}</ul>
    </section>
  );
}
