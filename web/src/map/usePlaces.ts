import { useEffect, useState } from "react";
import {
  type Place,
  placeQuery,
  placeQueryDelayMs,
  placeQueryMinLength,
  searchPlaces,
} from "./places";

export type PlaceSearch =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; places: Place[]; fuzzy: boolean };

// The places that match what the visitor types, asked of swisstopo once they
// have typed enough and paused. A newer query cancels the one before.
export function usePlaces(query: string): PlaceSearch {
  const text = placeQuery(query);
  const wanted = text.length >= placeQueryMinLength;
  const [search, setSearch] = useState<PlaceSearch>({ status: "idle" });

  useEffect(() => {
    if (!wanted) {
      setSearch({ status: "idle" });
      return;
    }
    setSearch({ status: "loading" });
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      searchPlaces(text, controller.signal).then(
        ({ places, fuzzy }) => {
          if (!controller.signal.aborted) {
            setSearch({ status: "ready", places, fuzzy });
          }
        },
        (cause: unknown) => {
          if (!controller.signal.aborted) {
            console.error("Searching places failed:", cause);
            setSearch({ status: "error" });
          }
        },
      );
    }, placeQueryDelayMs);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [text, wanted]);

  return wanted ? search : { status: "idle" };
}
