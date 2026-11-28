import { describe, expect, test } from "bun:test";
import { describePeriod, history } from "./describe";
import { type Period, stationOnMapPath, stationPath } from "./stations";

// Periods shaped like those of GET /v1/stations/{id}, the latest first.
const older: Period = {
  from: "2026-11-23T22:01:35Z",
  to: "2026-11-24T22:01:55Z",
  kind: "outdoor",
  powerClass: "medium",
  technologies: ["4G", "5G"],
  adaptive: true,
  permit: { type: "site_data_sheet", date: "2025-05-07" },
  installationLimitValue: 5,
};
const newer: Period = {
  ...older,
  from: "2026-11-24T22:01:55Z",
  to: null,
  powerClass: "high",
};

describe("describePeriod", () => {
  test("says each value in the visitor's language", () => {
    // The decimal separator is left out: Bun's ICU data differs from the
    // browsers'.
    expect(describePeriod("fr", older)).toMatchObject({
      kind: "Extérieur, plus de 6 W",
      powerClass: "Moyenne, jusqu’à 5000 W",
      technologies: "4G, 5G",
      adaptive: "Oui, au moins en partie",
      permit: "Fiche de données spécifique au site du 7 mai 2025",
    });
    expect(describePeriod("fr", older).limitValue).toMatch(/^5.0 V\/m$/);
  });

  test("says what is not stated", () => {
    const text = describePeriod("en", {
      ...older,
      technologies: [],
      adaptive: null,
      permit: null,
      installationLimitValue: null,
    });
    expect(text.technologies).toBe("None recognised");
    expect(text.adaptive).toBe("Not stated");
    expect(text.permit).toBe("Not stated");
    expect(text.limitValue).toBe("Not stated");
  });
});

describe("history", () => {
  test("lists only what changed, the latest first", () => {
    expect(history("en", [newer, older], true)).toEqual([
      {
        type: "period",
        from: newer.from,
        to: null,
        changes: [
          {
            label: "sheet.powerClass",
            before: "Medium, up to 5,000 W",
            after: "High, above 5,000 W",
          },
        ],
        first: false,
      },
      {
        type: "period",
        from: older.from,
        to: older.to,
        changes: [],
        first: true,
      },
    ]);
  });

  test("shows when the site was not published", () => {
    const back: Period = { ...older, from: "2026-11-26T22:01:00Z", to: null };
    const entries = history("en", [back, older], true);
    expect(entries.map((entry) => entry.type)).toEqual([
      "period",
      "gap",
      "period",
    ]);
    expect(entries[1]).toEqual({
      type: "gap",
      from: "2026-11-24T22:01:55Z",
      to: "2026-11-26T22:01:00Z",
    });
    expect(entries[0]).toMatchObject({ changes: [], first: false });
  });

  test("does not call the oldest period shown the first when more exist", () => {
    expect(history("en", [newer], false)).toEqual([
      { type: "period", from: newer.from, to: null, changes: [], first: false },
    ]);
  });
});

describe("station addresses", () => {
  test("encode the identifier, spaces and slashes included", () => {
    expect(stationPath("de", "2636501_1240299_Swisscom OFTR")).toBe(
      "/de/stations/2636501_1240299_Swisscom%20OFTR",
    );
    expect(stationPath("en", "1_2_A/B?#")).toBe("/en/stations/1_2_A%2FB%3F%23");
  });

  test("open the map at the station with its sheet", () => {
    expect(
      stationOnMapPath("fr", "2636501_1240299_Swisscom OFTR", {
        lat: 47.3125586,
        lon: 7.9213609,
      }),
    ).toBe(
      "/fr/?station=2636501_1240299_Swisscom%20OFTR#map=16/47.3125586/7.9213609",
    );
  });
});
