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
// have typed enough and paused. A newer query cancels the one before, and the
// answer to an older one is never shown.
export function usePlaces(query: string): PlaceSearch {
  const text = placeQuery(query);
  const wanted = text.length >= placeQueryMinLength;
  const [answer, setAnswer] = useState<{ text: string; search: PlaceSearch }>({
    text: "",
    search: { status: "idle" },
  });

  useEffect(() => {
    // Coming back to an earlier query asks again rather than show its old
    // answer.
    setAnswer({ text: "", search: { status: "idle" } });
    if (!wanted) {
      return;
    }
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      searchPlaces(text, controller.signal).then(
        ({ places, fuzzy }) => {
          if (!controller.signal.aborted) {
            setAnswer({ text, search: { status: "ready", places, fuzzy } });
          }
        },
        (cause: unknown) => {
          if (!controller.signal.aborted) {
            console.error("Searching places failed:", cause);
            setAnswer({ text, search: { status: "error" } });
          }
        },
      );
    }, placeQueryDelayMs);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [text, wanted]);

  if (!wanted) {
    return { status: "idle" };
  }
  return answer.text === text ? answer.search : { status: "loading" };
}
