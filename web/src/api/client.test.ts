import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { apiUrl, createApiClient, defaultApiUrl } from "./client";

describe("apiUrl", () => {
  test("defaults to the local API", () => {
    expect(apiUrl(undefined)).toBe(defaultApiUrl);
    expect(apiUrl("")).toBe(defaultApiUrl);
  });

  test("accepts an http or https origin", () => {
    expect(apiUrl("https://api.example.org")).toBe("https://api.example.org");
    expect(apiUrl("https://api.example.org/")).toBe("https://api.example.org");
    expect(apiUrl("http://127.0.0.1:8080")).toBe("http://127.0.0.1:8080");
  });

  test("refuses anything else", () => {
    for (const value of [
      "api.example.org",
      "ftp://api.example.org",
      "file:///etc/passwd",
      "https://user:secret@api.example.org",
      "https://api.example.org/v1",
      "https://api.example.org/?x=1",
      "https://api.example.org/#top",
    ]) {
      expect(() => apiUrl(value)).toThrow();
    }
  });

  test("never repeats the credentials it refuses", () => {
    expect(() => apiUrl("https://user:secret@api.example.org")).toThrow(
      /^(?!.*secret)/,
    );
  });
});

describe("createApiClient", () => {
  // An API on this machine: the snapshots answer, the counts send the
  // client elsewhere, and the list of stations never answers.
  const snapshots = {
    attribution: "Source: Federal Office of Communications OFCOM",
    snapshots: [],
    total: 0,
    offset: 0,
    limit: 100,
  };
  let api: ReturnType<typeof Bun.serve>;
  let elsewhere = 0;
  beforeAll(() => {
    api = Bun.serve({
      hostname: "127.0.0.1",
      port: 0,
      fetch(request) {
        const { pathname } = new URL(request.url);
        if (pathname === "/v1/snapshots") {
          return Response.json(snapshots);
        }
        if (pathname === "/v1/stations/counts") {
          return new Response(null, {
            status: 302,
            headers: { Location: "/elsewhere" },
          });
        }
        if (pathname === "/elsewhere") {
          elsewhere++;
          return Response.json({ total: 1 });
        }
        return new Promise<Response>(() => {});
      },
    });
  });
  afterAll(() => api.stop(true));

  test("calls the API at its origin", async () => {
    const client = createApiClient(api.url.origin, 1_000);
    const { data, response } = await client.GET("/v1/snapshots");
    expect(response.status).toBe(200);
    expect(data).toEqual(snapshots);
  });

  test("never follows a redirect", async () => {
    const client = createApiClient(api.url.origin, 1_000);
    await expect(client.GET("/v1/stations/counts")).rejects.toThrow(
      /redirect/i,
    );
    expect(elsewhere).toBe(0);
  });

  test("gives up after its timeout", async () => {
    const client = createApiClient(api.url.origin, 100);
    const started = performance.now();
    await expect(client.GET("/v1/stations")).rejects.toThrow(/time/i);
    expect(performance.now() - started).toBeLessThan(2_000);
  });
});
