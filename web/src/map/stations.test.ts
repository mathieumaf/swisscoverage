import { describe, expect, test } from "bun:test";
import {
  localizeLabel,
  lv95Of,
  type StationCollection,
  stationsAt,
  toMapData,
} from "./stations";

describe("toMapData", () => {
  test("keeps the position, the identifier, the label and the operator", () => {
    const collection: StationCollection = {
      type: "FeatureCollection",
      attribution: "Source: Federal Office of Communications OFCOM",
      snapshot: {
        publishedAt: "2026-11-24T22:01:55Z",
        sha256: "0".repeat(64),
        sites: 1,
      },
      features: [
        {
          type: "Feature",
          id: "2683328_1247916_Sunrise ZE416-1",
          geometry: { type: "Point", coordinates: [7.4474, 46.948] },
          properties: {
            name: "Sunrise ZE416-1",
            operator: "sunrise",
            kind: "outdoor",
            powerClass: "medium",
            technologies: ["4G", "5G"],
          },
        },
      ],
    };
    expect(toMapData(collection)).toEqual({
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          geometry: { type: "Point", coordinates: [7.4474, 46.948] },
          properties: {
            id: "2683328_1247916_Sunrise ZE416-1",
            name: "Sunrise ZE416-1",
            operator: "sunrise",
          },
        },
      ],
    });
  });
});

describe("localizeLabel", () => {
  const liberty = [
    "case",
    ["has", "name:nonlatin"],
    ["concat", ["get", "name:latin"], " ", ["get", "name:nonlatin"]],
    ["coalesce", ["get", "name_en"], ["get", "name"]],
  ];

  test("takes the name in the visitor's language, then the local one", () => {
    expect(localizeLabel(liberty, "de")).toEqual([
      "case",
      ["has", "name:nonlatin"],
      ["concat", ["get", "name:latin"], " ", ["get", "name:nonlatin"]],
      ["coalesce", ["get", "name:de"], ["get", "name"]],
    ]);
  });

  test("leaves English and other expressions as they are", () => {
    expect(localizeLabel(liberty, "en")).toEqual(liberty);
    expect(localizeLabel(["to-string", ["get", "ref"]], "fr")).toEqual([
      "to-string",
      ["get", "ref"],
    ]);
    expect(localizeLabel("{name}", "it")).toBe("{name}");
  });
});

describe("stationsAt", () => {
  test("lists each station under a click once", () => {
    const salt = {
      id: "a_b_Salt GE_2203A",
      name: "Salt GE_2203A",
      operator: "salt",
    };
    const swisscom = {
      id: "a_b_Swisscom ATHE",
      name: "Swisscom ATHE",
      operator: "swisscom",
    };
    expect(
      stationsAt([
        { properties: salt },
        { properties: swisscom },
        { properties: salt },
      ]),
    ).toEqual([salt, swisscom] as never);
  });

  test("skips what is not a station", () => {
    expect(
      stationsAt([
        { properties: null },
        { properties: { cluster_id: 3, point_count: 12 } },
      ]),
    ).toEqual([]);
  });

  test("lists an operator it does not know as other", () => {
    expect(
      stationsAt([
        { properties: { id: "x", name: "X", operator: "newcomer" } },
        { properties: { id: "y", name: "Y", operator: "toString" } },
      ]),
    ).toEqual([
      { id: "x", name: "X", operator: "other" },
      { id: "y", name: "Y", operator: "other" },
    ]);
  });
});

describe("lv95Of", () => {
  test("reads the position in an identifier", () => {
    expect(lv95Of("2497760_1121940_Salt GE_2203A")).toBe(
      "LV95 2497760, 1121940",
    );
  });
});
