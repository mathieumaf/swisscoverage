import { afterAll, beforeAll, describe, expect, spyOn, test } from "bun:test";
import { dev } from "astro";
import type { StationHistory } from "./map/stations";

// The page of a station, rendered by Astro's dev server against an API that
// this test serves on this machine with answers written here.

const zurich: StationHistory = {
  attribution: "Source: Federal Office of Communications OFCOM",
  id: "2683328_1247916_Sunrise ZH601-1",
  name: "Sunrise ZH601-1",
  code: "ZH601-1",
  operator: "sunrise",
  lv95: { east: 2683328, north: 1247916 },
  position: { lat: 47.3769, lon: 8.5417 },
  published: true,
  firstSeen: "2026-11-23T22:01:35Z",
  total: 2,
  offset: 0,
  limit: 1000,
  periods: [
    {
      from: "2026-11-24T22:01:55Z",
      to: null,
      kind: "outdoor",
      powerClass: "high",
      technologies: ["4G", "5G"],
      adaptive: true,
      permit: { type: "site_data_sheet", date: "2025-05-07" },
      installationLimitValue: 5,
    },
    {
      from: "2026-11-23T22:01:35Z",
      to: "2026-11-24T22:01:55Z",
      kind: "outdoor",
      powerClass: "medium",
      technologies: ["4G", "5G"],
      adaptive: true,
      permit: { type: "site_data_sheet", date: "2025-05-07" },
      installationLimitValue: 5,
    },
  ],
};

// A label with markup: the page must show it as text.
const markup: StationHistory = {
  ...zurich,
  id: "2600000_1200000_Other <script>alert(1)</script>",
  name: "Other <script>alert(1)</script>",
  code: "<script>alert(1)</script>",
  operator: "other",
};

// What the API received, one entry per request: the path and the query.
const received: string[] = [];
let api: ReturnType<typeof Bun.serve>;
let site: Awaited<ReturnType<typeof dev>>;
let previousApiUrl: string | undefined;

beforeAll(async () => {
  api = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      const url = new URL(request.url);
      received.push(url.pathname + url.search);
      const id = decodeURIComponent(url.pathname.replace("/v1/stations/", ""));
      for (const station of [zurich, markup]) {
        if (id === station.id) {
          return Response.json(station);
        }
      }
      if (id.endsWith("_Broken")) {
        return Response.json(
          { status: 500, detail: "postgres://reader:hunter2@db.internal" },
          {
            status: 500,
            headers: { "Content-Type": "application/problem+json" },
          },
        );
      }
      return Response.json(
        { status: 404, detail: "no station has this identifier" },
        {
          status: 404,
          headers: { "Content-Type": "application/problem+json" },
        },
      );
    },
  });
  // Read by the site on its first call to the API.
  previousApiUrl = process.env.SWISSCOVERAGE_API_URL;
  process.env.SWISSCOVERAGE_API_URL = api.url.origin;
  site = await dev({
    root: new URL("..", import.meta.url).pathname,
    logLevel: "silent",
    server: { host: "127.0.0.1", port: 0 },
  });
}, 60_000);

afterAll(async () => {
  await site?.stop();
  api?.stop(true);
  if (previousApiUrl === undefined) {
    delete process.env.SWISSCOVERAGE_API_URL;
  } else {
    process.env.SWISSCOVERAGE_API_URL = previousApiUrl;
  }
});

async function page(path: string): Promise<{ status: number; html: string }> {
  const response = await fetch(`http://127.0.0.1:${site.address.port}${path}`, {
    redirect: "manual",
  });
  return { status: response.status, html: await response.text() };
}

const heading = (html: string) => html.match(/<h1[^>]*>([^<]*)/)?.[1]?.trim();

describe("the page of a station", () => {
  test("shows what OFCOM publishes of it and its history", async () => {
    received.length = 0;
    const { status, html } = await page(
      "/fr/stations/2683328_1247916_Sunrise%20ZH601-1",
    );
    expect(status).toBe(200);
    expect(heading(html)).toBe("Sunrise ZH601-1");
    expect(html).toContain("<title>Sunrise ZH601-1 · swisscoverage</title>");
    expect(html).toContain("Dans les dernières données publiées");
    expect(html).toContain("Historique");
    // The latest period names what changed, the first says it is the first.
    expect(html).toContain("Première publication dans l’archive.");
    expect(html).toMatch(/<del[^>]*>Moyenne, jusqu’à 5000 W<\/del>/);
    expect(html).toContain("Voir sur la carte");
    // One call, to the path of the contract, with the station's id encoded.
    expect(received).toEqual([
      "/v1/stations/2683328_1247916_Sunrise%20ZH601-1?limit=1000",
    ]);
  });

  test("shows a label with markup as text", async () => {
    const { status, html } = await page(
      `/en/stations/${encodeURIComponent(markup.id)}`,
    );
    expect(status).toBe(200);
    // Inside a quoted attribute, such as the description, markup is text.
    const outsideAttributes = html.replace(/"[^"]*"/g, '""');
    expect(outsideAttributes).not.toContain("<script>alert(1)</script>");
    expect(heading(html)).toBe("Other &lt;script&gt;alert(1)&lt;/script&gt;");
    expect(html).toContain(
      "<title>Other &lt;script&gt;alert(1)&lt;/script&gt; · swisscoverage</title>",
    );
  });

  test("answers 404 for a station the API does not know", async () => {
    const { status, html } = await page("/fr/stations/1_2_Nothing");
    expect(status).toBe(404);
    expect(html).toContain('<html lang="fr"');
  });

  test("answers 404 without calling the API for an address that is no id", async () => {
    received.length = 0;
    for (const path of [
      "/fr/stations/Sunrise%20ZH601-1",
      "/fr/stations/1_2_",
      "/xx/stations/2683328_1247916_Sunrise%20ZH601-1",
    ]) {
      const { status } = await page(path);
      expect({ path, status }).toEqual({ path, status: 404 });
    }
    expect(received).toEqual([]);
  });

  test("says the station cannot be shown when the API fails, without why", async () => {
    const logged = spyOn(console, "error").mockImplementation(() => {});
    try {
      const { status, html } = await page("/de/stations/1_2_Broken");
      expect(status).toBe(503);
      expect(heading(html)).toBe("Anlage nicht verfügbar");
      expect(html).not.toContain("hunter2");
      expect(html).not.toContain("500");
    } finally {
      logged.mockRestore();
    }
  });
});
