import { describe, expect, test } from "bun:test";
import {
  allFilters,
  hasMapPosition,
  including,
  isEveryStationShown,
  matches,
  normalizeLabel,
  readFilters,
  searchStations,
  writeFilters,
} from "./filters";
import type { Station } from "./stations";

function station(overrides: Partial<Station>): Station {
  return {
    id: "2683328_1247916_Sunrise ZH601-1",
    name: "Sunrise ZH601-1",
    operator: "sunrise",
    kind: "outdoor",
    powerClass: "medium",
    technologies: ["4G", "5G"],
    coordinates: [8.54, 47.37],
    ...overrides,
  };
}

describe("matches", () => {
  test("shows every station when nothing is filtered", () => {
    const filters = allFilters();
    expect(isEveryStationShown(filters)).toBe(true);
    expect(matches(station({}), filters)).toBe(true);
    expect(matches(station({ technologies: [] }), filters)).toBe(true);
  });

  test("keeps a station whose operator, kind and power class are kept", () => {
    const filters = allFilters();
    filters.operators = new Set(["swisscom"]);
    expect(matches(station({}), filters)).toBe(false);
    expect(matches(station({ operator: "swisscom" }), filters)).toBe(true);

    filters.kinds = new Set(["indoor"]);
    expect(matches(station({ operator: "swisscom" }), filters)).toBe(false);

    const power = allFilters();
    power.powerClasses = new Set(["high"]);
    expect(matches(station({}), power)).toBe(false);
    expect(matches(station({ powerClass: "high" }), power)).toBe(true);
  });

  test("keeps a station with at least one kept technology", () => {
    const filters = allFilters();
    filters.technologies = new Set(["5G"]);
    expect(isEveryStationShown(filters)).toBe(false);
    expect(matches(station({}), filters)).toBe(true);
    expect(matches(station({ technologies: ["2G", "4G"] }), filters)).toBe(
      false,
    );
    expect(matches(station({ technologies: [] }), filters)).toBe(false);
  });
});

describe("matches with values the website does not know yet", () => {
  test("lets them through while their vocabulary is not filtered", () => {
    const newcomer = station({
      operator: "newcomer" as Station["operator"],
      kind: "rooftop" as Station["kind"],
    });
    expect(matches(newcomer, allFilters())).toBe(true);
    const filters = allFilters();
    filters.operators = new Set(["swisscom"]);
    expect(matches(newcomer, filters)).toBe(false);
  });
});

describe("including", () => {
  test("widens the filters just enough to show a station", () => {
    const filters = allFilters();
    filters.operators = new Set(["swisscom"]);
    filters.technologies = new Set(["2G"]);
    const sunrise = station({ technologies: ["4G", "5G"] });
    const widened = including(filters, sunrise);
    expect(matches(sunrise, widened)).toBe(true);
    expect([...widened.operators]).toEqual(["swisscom", "sunrise"]);
    expect([...widened.technologies]).toEqual(["2G", "4G", "5G"]);
    expect(widened.kinds).toEqual(filters.kinds);
  });

  test("leaves alone a vocabulary that already shows the station", () => {
    const filters = allFilters();
    filters.operators = new Set(["swisscom"]);
    filters.technologies = new Set(["5G"]);
    const widened = including(filters, station({ technologies: ["4G", "5G"] }));
    expect([...widened.technologies]).toEqual(["5G"]);
    expect(widened.technologies).toBe(filters.technologies);
    expect([...widened.operators]).toEqual(["swisscom", "sunrise"]);
  });

  test("keeps every value for a value the panel does not know", () => {
    const filters = allFilters();
    filters.operators = new Set(["swisscom"]);
    const widened = including(
      filters,
      station({ operator: "newcomer" as Station["operator"] }),
    );
    expect(widened.operators.size).toBe(5);
    expect(widened.operators.has("newcomer" as Station["operator"])).toBe(
      false,
    );
  });

  test("keeps the filters when the station already shows", () => {
    const filters = allFilters();
    expect(including(filters, station({}))).toBe(filters);
  });

  test("keeps every technology for a station without one", () => {
    const filters = allFilters();
    filters.technologies = new Set(["5G"]);
    const widened = including(filters, station({ technologies: [] }));
    expect(widened.technologies.size).toBe(4);
  });
});

describe("hasMapPosition", () => {
  test("reads MapLibre's hash", () => {
    expect(hasMapPosition("#map=11/46.948/7.447")).toBe(true);
    expect(hasMapPosition("#map=15/47.15737/7.41774/0/0")).toBe(true);
  });

  test("refuses what MapLibre would not use", () => {
    for (const hash of [
      "",
      "#map=bogus",
      "#othermap=11/46.9/7.4",
      "#map=11/46.9",
      "#map=11/146.9/7.4",
      "#map=40/46.9/7.4",
      "#map=23/46.9/7.4",
      "#map=11/46.9/7.4/181",
      "#map=11/46.9/7.4/0/61",
    ]) {
      expect(hasMapPosition(hash)).toBe(false);
    }
  });
});

describe("readFilters and writeFilters", () => {
  test("an address without filters keeps everything", () => {
    expect(readFilters(new URLSearchParams(""))).toEqual(allFilters());
  });

  test("reads the kept values and ignores unknown ones", () => {
    const filters = readFilters(
      new URLSearchParams(
        "operator=salt,swisscom,nobody&technology=5G&kind=&power=high,toString",
      ),
    );
    expect([...filters.operators]).toEqual(["salt", "swisscom"]);
    expect([...filters.technologies]).toEqual(["5G"]);
    expect([...filters.kinds]).toEqual([]);
    expect([...filters.powerClasses]).toEqual(["high"]);
  });

  test("writes only what is filtered, in the panel's order, and keeps the rest", () => {
    const filters = allFilters();
    filters.operators = new Set(["salt", "swisscom"]);
    filters.technologies = new Set(["5G"]);
    const search = new URLSearchParams("station=abc&kind=indoor");
    writeFilters(filters, search);
    expect(search.toString()).toBe(
      "station=abc&operator=swisscom%2Csalt&technology=5G",
    );
    expect(readFilters(search)).toEqual(filters);
  });
});

describe("searchStations", () => {
  const stations = [
    station({ id: "a", name: "Sunrise ZH601-1" }),
    station({ id: "b", name: "Swisscom ZHHB" }),
    station({ id: "c", name: "Salt ZH_0601A" }),
    station({ id: "d", name: "Swisscom GENÈVE-GARE" }),
  ];

  test("finds a label whatever its spaces, dashes, case and accents", () => {
    expect(normalizeLabel("Sunrise ZH601-1")).toBe("sunrisezh6011");
    expect(searchStations(stations, "zh 601", 10).map((s) => s.id)).toEqual([
      "a",
    ]);
    expect(searchStations(stations, "geneve", 10).map((s) => s.id)).toEqual([
      "d",
    ]);
  });

  test("puts the earliest match first and stops at the limit", () => {
    expect(searchStations(stations, "zh", 10).map((s) => s.id)).toEqual([
      "c",
      "a",
      "b",
    ]);
    expect(searchStations(stations, "zh", 2)).toHaveLength(2);
  });

  test("finds nothing for an empty query", () => {
    expect(searchStations(stations, " - ", 10)).toEqual([]);
  });
});
