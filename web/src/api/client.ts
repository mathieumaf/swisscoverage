import createClient from "openapi-fetch";
import type { paths } from "./schema";

// The API that `swisscoverage serve` answers on this machine by default.
export const defaultApiUrl = "http://127.0.0.1:8080";

// How long the server waits for the API before it gives up on a request.
const timeoutMs = 5_000;

// The origin of the API, read from SWISSCOVERAGE_API_URL. It is the only
// address the server calls: nothing a visitor sends changes it. It must be an
// http or https origin, without a path, a query or credentials, since the
// paths of the contract already start with /v1.
export function apiUrl(value: string | undefined): string {
  if (value === undefined || value === "") {
    return defaultApiUrl;
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error("SWISSCOVERAGE_API_URL is not a URL");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("SWISSCOVERAGE_API_URL must use http or https");
  }
  if (url.username !== "" || url.password !== "") {
    throw new Error("SWISSCOVERAGE_API_URL must not contain credentials");
  }
  if (url.pathname !== "/" || url.search !== "" || url.hash !== "") {
    throw new Error(
      "SWISSCOVERAGE_API_URL must be an origin, such as https://api.example.org",
    );
  }
  return url.origin;
}

// A client of the public API, typed by the contract in openapi.json.
export function createApiClient(baseUrl: string) {
  return createClient<paths>({
    baseUrl,
    fetch: (request) =>
      fetch(request, { signal: AbortSignal.timeout(timeoutMs) }),
  });
}

let client: ReturnType<typeof createApiClient> | undefined;

// The client the pages use, created on first use so that a wrong address
// fails the request that needs it, with a message that says why.
export function api() {
  client ??= createApiClient(apiUrl(process.env.SWISSCOVERAGE_API_URL));
  return client;
}
