// The site's four languages. Each page lives under its language, as in
// /fr/, /de/, /it/ and /en/.
export const locales = ["fr", "de", "it", "en"] as const;

export type Locale = (typeof locales)[number];

// For a visitor whose browser asks for none of the four languages.
export const defaultLocale: Locale = "en";

export function isLocale(value: string | undefined): value is Locale {
  return (locales as readonly string[]).includes(value ?? "");
}

// The tag given to Intl, so that numbers and dates follow Swiss conventions in
// every language.
const intlTags: Record<Locale, string> = {
  fr: "fr-CH",
  de: "de-CH",
  it: "it-CH",
  en: "en-CH",
};

export function formatNumber(locale: Locale, value: number): string {
  return new Intl.NumberFormat(intlTags[locale]).format(value);
}

// Dates are shown as they were in Switzerland: a snapshot published at
// 23:30 UTC on 24 November belongs to 25 November.
export function formatDate(locale: Locale, value: Date): string {
  return new Intl.DateTimeFormat(intlTags[locale], {
    dateStyle: "long",
    timeZone: "Europe/Zurich",
  }).format(value);
}

// The same address in another language: the first segment of the path is the
// language.
export function localizedPath(pathname: string, locale: Locale): string {
  const [, first, ...rest] = pathname.split("/");
  const tail = isLocale(first) ? rest : [first ?? "", ...rest];
  return `/${[locale, ...tail].join("/")}${tail.length === 0 ? "/" : ""}`;
}

// The language to show a visitor, from the Accept-Language header of their
// browser: the one of the four they rank highest, whatever the region, so
// that de-CH and de-DE both get German. A language ranked at q=0 is refused.
export function negotiateLocale(acceptLanguage: string | null): Locale {
  const ranked: { locale: Locale; quality: number; order: number }[] = [];
  for (const [order, part] of (acceptLanguage ?? "").split(",").entries()) {
    const [tag = "", ...params] = part.trim().split(";");
    const language = tag.trim().toLowerCase().split("-")[0];
    if (!isLocale(language)) {
      continue;
    }
    let quality = 1;
    for (const param of params) {
      const [name, value] = param.trim().split("=");
      if (name?.trim().toLowerCase() === "q") {
        quality = Number(value);
      }
    }
    if (Number.isFinite(quality) && quality > 0 && quality <= 1) {
      ranked.push({ locale: language, quality, order });
    }
  }
  ranked.sort((a, b) => b.quality - a.quality || a.order - b.order);
  return ranked[0]?.locale ?? defaultLocale;
}
