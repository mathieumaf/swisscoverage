import { afterEach, describe, expect, test } from "bun:test";
import {
  labelText,
  placeQuery,
  placeSearchAddress,
  searchPlaces,
  toPlaces,
} from "./places";

// Results shaped like the answers of swisstopo's location search
// (docs/data-sources.md), with the fields the site reads.
const address = {
  id: 1006646,
  weight: 7,
  attrs: {
    origin: "address",
    label: "Bundesplatz 3 <b>3011 Bern</b>",
    lat: 46.946773529052734,
    lon: 7.444191932678223,
    geom_st_box2d: "BOX(7.444191 46.946774,7.444191 46.946774)",
  },
};
const municipality = {
  id: 5586,
  weight: 1,
  attrs: {
    origin: "gg25",
    label: "<b>Lausanne (VD)</b>",
    lat: 46.52865219116211,
    lon: 6.638521671295166,
    geom_st_box2d: "BOX(6.582741 46.454464,6.722472 46.602974)",
  },
};

describe("placeSearchAddress", () => {
  test("asks for addresses, postcodes and municipalities, in WGS84", () => {
    const url = placeSearchAddress("  Bundesplatz 3   Bern ");
    expect(url.origin + url.pathname).toBe(
      "https://api3.geo.admin.ch/rest/services/ech/SearchServer",
    );
    expect(Object.fromEntries(url.searchParams)).toEqual({
      type: "locations",
      origins: "address,zipcode,gg25",
      sr: "4326",
      limit: "5",
      searchText: "Bundesplatz 3 Bern",
    });
  });

  test("sends at most ten words", () => {
    expect(placeQuery("a b c d e f g h i j k l")).toBe("a b c d e f g h i j");
  });
});

describe("toPlaces", () => {
  test("keeps an address at its point and a municipality with its extent", () => {
    expect(toPlaces({ results: [address, municipality] })).toEqual({
      fuzzy: false,
      places: [
        {
          key: "address:0",
          origin: "address",
          label: "Bundesplatz 3 3011 Bern",
          center: [7.444191932678223, 46.946773529052734],
        },
        {
          key: "gg25:1",
          origin: "gg25",
          label: "Lausanne (VD)",
          center: [6.638521671295166, 46.52865219116211],
          bounds: [
            [6.582741, 46.454464],
            [6.722472, 46.602974],
          ],
        },
      ],
    });
  });

  test("says when the service guessed", () => {
    expect(toPlaces({ fuzzy: "true", results: [] }).fuzzy).toBe(true);
  });

  test("leaves out what is not of the expected shape", () => {
    const attrs = address.attrs;
    const results = [
      null,
      { attrs: null },
      { attrs: { ...attrs, origin: "gazetteer" } },
      { attrs: { ...attrs, lat: "46.9" } },
      { attrs: { ...attrs, lon: Number.NaN } },
      { attrs: { ...attrs, lat: 52.52, lon: 13.4 } },
      { attrs: { ...attrs, label: 3 } },
      { attrs: { ...attrs, label: "<b></b>" } },
    ];
    expect(toPlaces({ results }).places).toEqual([]);
  });

  test("keeps a place whose extent is malformed or outside the map, without it", () => {
    for (const box of [
      "BOX(6.7 46.6,6.5 46.4)",
      "BOX(1 46.4,6.7 46.6)",
      "POLYGON((6.5 46.4))",
      42,
    ]) {
      const [place] = toPlaces({
        results: [
          {
            ...municipality,
            attrs: { ...municipality.attrs, geom_st_box2d: box },
          },
        ],
      }).places;
      expect(place?.label).toBe("Lausanne (VD)");
      expect(place?.bounds).toBeUndefined();
    }
  });

  test("keeps five places at most", () => {
    const results = Array.from({ length: 8 }, () => address);
    expect(toPlaces({ results }).places).toHaveLength(5);
  });

  test("refuses a document that is not an answer", () => {
    for (const answer of [null, [], "results", { results: {} }]) {
      expect(() => toPlaces(answer)).toThrow();
    }
  });
});

describe("labelText", () => {
  test("keeps the text of the label, never its markup", () => {
    expect(labelText("Bundesplatz 3 <b>3011 Bern</b>")).toBe(
      "Bundesplatz 3 3011 Bern",
    );
    expect(labelText('<img src=x onerror="alert(1)">Genève')).toBe("Genève");
    expect(labelText("A &amp; B &lt;b&gt;")).toBe("A & B <b>");
  });

  test("drops the mark of a street without a number", () => {
    expect(
      labelText("Route de Lausanne # <b>1032 Romanel-sur-Lausanne</b>"),
    ).toBe("Route de Lausanne 1032 Romanel-sur-Lausanne");
  });
});

describe("searchPlaces", () => {
  const realFetch = globalThis.fetch;
  const realAny = AbortSignal.any;
  afterEach(() => {
    globalThis.fetch = realFetch;
    AbortSignal.any = realAny;
  });

  // Browsers before Safari 17.4 and Chrome 116 lack `AbortSignal.any`.
  function withoutAny() {
    // @ts-expect-error: removed as in an older browser.
    AbortSignal.any = undefined;
  }

  test("reads an answer without credentials or redirects", async () => {
    withoutAny();
    let options: RequestInit | undefined;
    globalThis.fetch = (async (_: unknown, init?: RequestInit) => {
      options = init;
      return Response.json({ results: [address] });
    }) as typeof fetch;
    const { places } = await searchPlaces(
      "Bundesplatz 3",
      new AbortController().signal,
    );
    expect(places.map((place) => place.label)).toEqual([
      "Bundesplatz 3 3011 Bern",
    ]);
    expect(options?.credentials).toBe("omit");
    expect(options?.redirect).toBe("error");
  });

  test("stops when the caller gives up", async () => {
    withoutAny();
    globalThis.fetch = ((_: unknown, init?: RequestInit) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () =>
          reject(new DOMException("Aborted", "AbortError")),
        );
      })) as typeof fetch;
    const caller = new AbortController();
    const search = searchPlaces("Bern", caller.signal);
    caller.abort();
    await expect(search).rejects.toThrow("Aborted");
  });

  test("refuses an answer that is an error", async () => {
    globalThis.fetch = (async () =>
      new Response("busy", { status: 503 })) as unknown as typeof fetch;
    await expect(
      searchPlaces("Bern", new AbortController().signal),
    ).rejects.toThrow("503");
  });
});
