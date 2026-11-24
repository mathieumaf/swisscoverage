import { describe, expect, test } from "bun:test";
import sample from "./fixtures/mobile-stations.sample.json" with {
  type: "json",
};
import { OfcomFormatError, parseMobileStations } from "./mobile-stations.ts";

// The fixture holds nine real stations taken from the snapshot of 2026-11-23,
// one per shape the file is known to contain.
const { stations, warnings } = parseMobileStations(sample);
const byName = (name: string) => {
  const station = stations.find((candidate) => candidate.name === name);
  if (station === undefined) throw new Error(`No station named ${name}`);
  return station;
};

/** A copy of the fixture's first feature with some properties replaced. */
function documentWith(properties: Record<string, unknown>) {
  const [feature] = sample.features;
  return {
    ...sample,
    features: [
      { ...feature, properties: { ...feature?.properties, ...properties } },
    ],
  };
}

describe("parseMobileStations", () => {
  test("reads every station of the sample without warnings", () => {
    expect(stations).toHaveLength(sample.features.length);
    expect(warnings).toEqual([]);
  });

  test("reads a typical operator site", () => {
    expect(byName("Swisscom BIFO")).toEqual({
      name: "Swisscom BIFO",
      operator: "swisscom",
      code: "BIFO",
      kind: "outdoor",
      powerClass: "medium",
      technologies: ["4G", "5G"],
      adaptive: true,
      permit: { type: "site_data_sheet", date: "2021-06-03" },
      installationLimitValue: 5,
      lv95: { east: 2_676_878, north: 1_246_069 },
    });
  });

  test("separates the operator from the station code", () => {
    expect(byName("Salt TI_3787A")).toMatchObject({
      operator: "salt",
      code: "TI_3787A",
    });
    expect(byName("Sunrise BN375-1")).toMatchObject({
      operator: "sunrise",
      code: "BN375-1",
    });
    expect(byName("SBB AU-WAE-02202")).toMatchObject({
      operator: "sbb",
      code: "AU-WAE-02202",
    });
  });

  test("groups foreign border stations under other", () => {
    expect(byName("DB-BSBB")).toMatchObject({
      operator: "other",
      code: "DB-BSBB",
    });
  });

  test("reads each kind of site", () => {
    expect(byName("Swisscom BIFO").kind).toBe("outdoor");
    expect(byName("SBB AU-WAE-02202").kind).toBe("outdoor_low_power");
    expect(byName("Swisscom BEAL").kind).toBe("indoor");
    expect(byName("Salt FR_7401A").kind).toBe("tunnel");
    expect(byName("Swisscom XTHR").kind).toBe("temporary");
  });

  test("accepts a site published without a type", () => {
    expect(byName("Sunrise BN375-1").kind).toBe("unknown");
  });

  test("reads each power class", () => {
    expect(byName("Swisscom BEAL").powerClass).toBe("very_low");
    expect(byName("Swisscom DIBH").powerClass).toBe("low");
    expect(byName("Swisscom BIFO").powerClass).toBe("medium");
    expect(byName("Salt TI_3787A").powerClass).toBe("high");
  });

  test("reads the technologies in generation order", () => {
    expect(byName("SBB AU-WAE-02202").technologies).toEqual(["2G"]);
    expect(byName("Salt TI_3787A").technologies).toEqual(["3G", "4G", "5G"]);
  });

  test("reads the adaptive flag", () => {
    expect(byName("Swisscom BEAL").adaptive).toBe(true);
    expect(byName("Swisscom DIBH").adaptive).toBe(false);
  });

  test("distinguishes a data sheet, an exemption and no permit", () => {
    expect(byName("Swisscom DIBH").permit).toEqual({
      type: "site_data_sheet",
      date: "2018-09-18",
    });
    expect(byName("Salt FR_7401A").permit).toEqual({ type: "exempt" });
    expect(byName("Swisscom BEAL").permit).toBeNull();
  });

  test("reads the installation limit value, or null when left blank", () => {
    expect(byName("Swisscom DIBH").installationLimitValue).toBe(6);
    // Published as "Installation limit value - V/m".
    expect(byName("SBB AU-WAE-02202").installationLimitValue).toBeNull();
    // Published as an empty string.
    expect(byName("Swisscom BEAL").installationLimitValue).toBeNull();
  });

  test("keeps a station with unrecognised labels and reports them", () => {
    const result = parseMobileStations(
      documentWith({
        typ_en: "Balloon",
        power_en: "Power class : enormous",
        techno_en: "Technology 6G",
        adaptiv_en: "Fully adaptive operation",
        bewilligung_en: "Pending",
        agw_en: "Installation limit value high",
      }),
    );

    expect(result.stations[0]).toMatchObject({
      name: "Swisscom BIFO",
      kind: "unknown",
      powerClass: "unknown",
      technologies: [],
      adaptive: true,
      permit: null,
      installationLimitValue: null,
    });
    expect(result.warnings).toEqual([
      'Swisscom BIFO: unrecognised type "Balloon"',
      'Swisscom BIFO: unrecognised power class "Power class : enormous"',
      'Swisscom BIFO: unrecognised technology "Technology 6G"',
      'Swisscom BIFO: unrecognised adaptive operation "Fully adaptive operation"',
      'Swisscom BIFO: unrecognised permit "Pending"',
      'Swisscom BIFO: unrecognised installation limit value "Installation limit value high"',
    ]);
  });

  test("rejects a document that is not a FeatureCollection", () => {
    expect(() => parseMobileStations({ type: "Feature" })).toThrow(
      OfcomFormatError,
    );
    expect(() => parseMobileStations(null)).toThrow(OfcomFormatError);
  });

  test("rejects coordinates in another reference system", () => {
    const wgs84 = {
      ...sample,
      crs: { type: "name", properties: { name: "EPSG:4326" } },
    };
    expect(() => parseMobileStations(wgs84)).toThrow(
      "Expected coordinates in EPSG:2056, found EPSG:4326.",
    );
    expect(() => parseMobileStations({ ...sample, crs: undefined })).toThrow(
      "found no CRS",
    );
  });

  test("rejects a feature without a station label or a position", () => {
    expect(() => parseMobileStations(documentWith({ station: 42 }))).toThrow(
      'Feature 0 has no "station" property.',
    );
    expect(() => parseMobileStations(documentWith({ station: "" }))).toThrow(
      "Feature 0 has an empty station label.",
    );
    const [feature] = sample.features;
    const withoutGeometry = {
      ...sample,
      features: [{ ...feature, geometry: null }],
    };
    expect(() => parseMobileStations(withoutGeometry)).toThrow(
      "Feature 0 (Swisscom BIFO) has no point geometry.",
    );
  });
});
