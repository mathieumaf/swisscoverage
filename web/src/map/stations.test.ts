import { describe, expect, test } from "bun:test";
import { localizeLabel, type StationCollection, toMapData } from "./stations";

describe("toMapData", () => {
  test("keeps the position, the identifier and the operator", () => {
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
