import { describe, expect, test } from "bun:test";
import { apiUrl, defaultApiUrl } from "./client";

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
