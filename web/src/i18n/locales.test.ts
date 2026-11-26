import { describe, expect, test } from "bun:test";
import {
  formatDate,
  formatNumber,
  isLocale,
  localizedPath,
  negotiateLocale,
} from "./locales";
import { translate } from "./translate";

describe("isLocale", () => {
  test("accepts the four languages only", () => {
    for (const locale of ["fr", "de", "it", "en"]) {
      expect(isLocale(locale)).toBe(true);
    }
    for (const value of ["", "rm", "FR", "fr-CH", "favicon.ico", undefined]) {
      expect(isLocale(value)).toBe(false);
    }
  });
});

describe("formatNumber", () => {
  // The separator itself comes from the ICU data of the runtime, which differs
  // between platforms: only its presence is checked.
  test("groups thousands", () => {
    for (const locale of ["fr", "de", "it", "en"] as const) {
      expect(formatNumber(locale, 22761)).toMatch(/^22\D761$/);
    }
  });
});

describe("formatDate", () => {
  test("gives the day in Switzerland, not in UTC", () => {
    const lateEvening = new Date("2026-11-24T23:30:00Z");
    expect(formatDate("en", lateEvening)).toBe("25 November 2026");
    expect(formatDate("de", lateEvening)).toBe("25. November 2026");
    expect(formatDate("fr", lateEvening)).toBe("25 novembre 2026");
    expect(formatDate("it", lateEvening)).toBe("25 novembre 2026");
  });
});

describe("localizedPath", () => {
  test("replaces the language of a path", () => {
    expect(localizedPath("/fr/", "de")).toBe("/de/");
    expect(localizedPath("/fr", "de")).toBe("/de/");
    expect(localizedPath("/it/stations/abc", "en")).toBe("/en/stations/abc");
  });

  test("adds a language to a path without one", () => {
    expect(localizedPath("/", "fr")).toBe("/fr/");
    expect(localizedPath("/unknown", "fr")).toBe("/fr/unknown");
  });
});

describe("translate", () => {
  test("fills each placeholder", () => {
    expect(
      translate("en", "home.snapshot", { count: "3", date: "1 May 2027" }),
    ).toBe("3 sites in the data published on 1 May 2027.");
  });

  test("leaves a placeholder without a value visible", () => {
    expect(translate("fr", "home.snapshot", { count: "3" })).toBe(
      "3 sites dans les données publiées le {date}.",
    );
  });
});

describe("negotiateLocale", () => {
  test("matches a language whatever its region", () => {
    expect(negotiateLocale("de-CH")).toBe("de");
    expect(negotiateLocale("fr-CH,fr;q=0.9,en;q=0.8")).toBe("fr");
    expect(negotiateLocale("IT-ch")).toBe("it");
  });

  test("takes the highest ranked language it has", () => {
    expect(negotiateLocale("rm,de;q=0.5,fr;q=0.8")).toBe("fr");
    expect(negotiateLocale("en;q=0.5, it ;q=0.9")).toBe("it");
  });

  test("keeps the browser's order between equal ranks", () => {
    expect(negotiateLocale("it,fr")).toBe("it");
    expect(negotiateLocale("de;q=0.7,fr;q=0.7")).toBe("de");
  });

  test("skips a language refused or ranked out of bounds", () => {
    expect(negotiateLocale("fr;q=0,de;q=0.1")).toBe("de");
    expect(negotiateLocale("fr;q=2,it;q=abc,de;q=0.3")).toBe("de");
  });

  test("falls back to the default language", () => {
    expect(negotiateLocale(null)).toBe("en");
    expect(negotiateLocale("")).toBe("en");
    expect(negotiateLocale("rm,ja;q=0.5,*")).toBe("en");
  });
});
