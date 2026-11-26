import {
  type ApiClient,
  apiUrl,
  createApiClient,
  serverTimeoutMs,
} from "./client";

// The origin of the API for this server and for the browsers it serves, from
// SWISSCOVERAGE_API_URL. Read on first use, so that a wrong address fails
// the request that needs it, with a message that says why.
export function serverApiUrl(): string {
  return apiUrl(process.env.SWISSCOVERAGE_API_URL);
}

let client: ApiClient | undefined;

// The client the pages use on the server.
export function api(): ApiClient {
  client ??= createApiClient(serverApiUrl(), serverTimeoutMs);
  return client;
}
