import { describe, expect, test } from "bun:test";
import { describePeriod, history, positionText } from "./describe";
import {
  type Period,
  stationIdFromPath,
  stationOnMapPath,
  stationPath,
} from "./stations";

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

  test("shows a small change of the limit value", () => {
    const close: Period = { ...newer, powerClass: "medium" };
    const entries = history(
      "en",
      [{ ...close, installationLimitValue: 5.0002 }, older],
      true,
    );
    expect(entries[0]).toMatchObject({
      changes: [
        { label: "sheet.limitValue", before: "5.0 V/m", after: "5.0002 V/m" },
      ],
    });
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

  test("read back the identifier they hold, reserved characters included", () => {
    for (const id of [
      "2636501_1240299_Swisscom OFTR",
      "1_2_A/B?#",
      "1_2_A%41",
      "1_2_A.html",
      "1_2_A+B,C&D:E",
    ]) {
      expect(stationIdFromPath(stationPath("it", id), "it")).toBe(id);
    }
    expect(
      stationIdFromPath("/it/stations/1_2_%E0%A4%A", "it"),
    ).toBeUndefined();
    expect(stationIdFromPath("/fr/stations/1_2_A", "it")).toBeUndefined();
    expect(stationIdFromPath("/it/stations/1_2_A/", "it")).toBe("1_2_A");
    expect(stationIdFromPath("/it/stations/1_2_A%2F", "it")).toBe("1_2_A/");
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

describe("positionText", () => {
  test("gives WGS84 to five decimals and LV95 as published", () => {
    expect(
      positionText({
        position: { lat: 46.9480012, lon: 7.4474 },
        lv95: { east: 2600640, north: 1199640.5 },
      }),
    ).toEqual({ wgs84: "46.94800, 7.44740", lv95: "LV95 2600640, 1199640.5" });
  });
});
