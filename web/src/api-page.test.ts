import { afterAll, beforeAll, describe, expect, spyOn, test } from "bun:test";
import { dev } from "astro";
import { reference } from "./api/contract";

// The page about the API, rendered by Astro's dev server. It calls no API:
// it is made of the contract and of the origin in SWISSCOVERAGE_API_URL,
// which its examples start with.

const origin = "http://127.0.0.1:8081";
let site: Awaited<ReturnType<typeof dev>>;
// What the test sets for the site, and what it restores after: no telemetry
// nor update check, which would call out and write the settings that make
// the dev server restart.
const environment = {
  SWISSCOVERAGE_API_URL: origin,
  ASTRO_TELEMETRY_DISABLED: "1",
  ASTRO_DISABLE_UPDATE_CHECK: "true",
};
const previous: Record<string, string | undefined> = {};

beforeAll(async () => {
  for (const [name, value] of Object.entries(environment)) {
    previous[name] = process.env[name];
    process.env[name] = value;
  }
  // A port chosen once: should the dev server restart, it comes back on it.
  const free = Bun.listen({
    hostname: "127.0.0.1",
    port: 0,
    socket: { data() {} },
  });
  const port = free.port;
  free.stop(true);
  site = await dev({
    root: new URL("..", import.meta.url).pathname,
    logLevel: "silent",
    server: { host: "127.0.0.1", port },
  });
}, 60_000);

afterAll(async () => {
  await site?.stop();
  for (const [name, value] of Object.entries(previous)) {
    if (value === undefined) {
      delete process.env[name];
    } else {
      process.env[name] = value;
    }
  }
});

async function page(path: string): Promise<{ status: number; html: string }> {
  const response = await fetch(`http://127.0.0.1:${site.address.port}${path}`, {
    redirect: "manual",
  });
  return { status: response.status, html: await response.text() };
}

const heading = (html: string) => html.match(/<h1[^>]*>([^<]*)/)?.[1]?.trim();

describe("the page about the API", () => {
  test("shows a first request to copy, against the configured API", async () => {
    const { status, html } = await page("/fr/api");
    expect(status).toBe(200);
    expect(html).toContain('<html lang="fr"');
    expect(heading(html)).toBe("L’API");
    expect(html).toContain(
      `curl -sS --compressed &#39;${origin}/v1/stations/search?lat=46.948&amp;lon=7.4474&amp;radius=300&#39;`,
    );
    expect(html).toContain(`href="${origin}/v1/openapi.json"`);
    // The page is in the navigation, marked as the current one.
    expect(html).toMatch(/href="\/fr\/api"[^>]*aria-current="page"/);
  });

  test("describes every endpoint and every type of the contract", async () => {
    const { html } = await page("/en/api");
    for (const endpoint of reference.endpoints) {
      expect(html).toContain(`id="endpoint-${endpoint.id}"`);
      for (const parameter of endpoint.parameters) {
        expect(html).toContain(
          `<code class="font-mono font-bold">${parameter.name}</code>`,
        );
      }
    }
    for (const schema of reference.schemas) {
      expect(html).toContain(`id="schema-${schema.name}"`);
    }
    expect(html).toContain("OFCOM");
    expect(html).toContain("https://opendata.swiss/en/terms-of-use/#terms_by");
  });

  test("exists in each language", async () => {
    for (const [path, title] of [
      ["/de/api", "Die API"],
      ["/it/api", "L’API"],
      ["/en/api", "The API"],
    ]) {
      const { status, html } = await page(path ?? "");
      expect({ path, status, title: heading(html) }).toEqual({
        path,
        status: 200,
        title,
      });
    }
    expect((await page("/xx/api")).status).toBe(404);
  });

  test("renders without examples when the API's address is not usable", async () => {
    const logged = spyOn(console, "error").mockImplementation(() => {});
    process.env.SWISSCOVERAGE_API_URL = "ftp://example.org";
    try {
      const { status, html } = await page("/en/api");
      expect(status).toBe(200);
      expect(html).toContain("The address of the API is not available");
      expect(html).not.toContain("curl ");
      expect(html).not.toContain("ftp://example.org");
    } finally {
      process.env.SWISSCOVERAGE_API_URL = origin;
      logged.mockRestore();
    }
  });
});
