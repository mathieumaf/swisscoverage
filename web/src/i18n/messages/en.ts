// The English messages are the reference: every other language must have the
// same keys, which the type of `Messages` enforces.
export const en = {
  "site.description":
    "Mobile network antennas in Switzerland, on an open map, with a public API.",
  "nav.language": "Language",
  "home.title": "Mobile network antennas in Switzerland",
  "home.intro":
    "An open map, a public API and an MCP server for the mobile network sites that OFCOM publishes. The map is being built.",
  "home.snapshot": "{count} sites in the data published on {date}.",
  "home.unavailable": "The data cannot be loaded right now.",
  "footer.source": "Source: Federal Office of Communications OFCOM",
  "footer.code": "Source code",
  "footer.licence": "Free software under the AGPL-3.0 licence.",
  "notFound.title": "Page not found",
  "notFound.text": "This address does not lead to any page.",
  "notFound.home": "Back to the home page",
} as const;

export type MessageKey = keyof typeof en;

export type Messages = { readonly [K in MessageKey]: string };
