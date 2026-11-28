import { describe, expect, test } from "bun:test";
import contract from "../../../openapi.json";
import {
  type Endpoint,
  exampleCommand,
  exampleUrl,
  type OpenApiDocument,
  readContract,
  reference,
  segments,
  typeName,
} from "./contract";

// The documentation of the API is what these functions read from the
// contract: the page shows each part of it, and nothing else.

const document = contract as unknown as OpenApiDocument;
const origin = "http://127.0.0.1:8080";

function endpoint(id: string): Endpoint {
  const found = reference.endpoints.find((item) => item.id === id);
  if (found === undefined) {
    throw new Error(`no endpoint ${id}`);
  }
  return found;
}

describe("the reference read from openapi.json", () => {
  test("has every operation of the contract, with every parameter", () => {
    const operations = Object.entries(document.paths).flatMap(
      ([path, methods]) =>
        Object.entries(methods).map(([method, operation]) => ({
          path,
          method: method.toUpperCase(),
          parameters: (operation.parameters ?? [])
            .map((parameter) => parameter.name)
            .filter((name) => name !== "If-None-Match"),
        })),
    );
    expect(operations.length).toBeGreaterThan(0);
    expect(
      reference.endpoints.map(({ path, method, parameters }) => ({
        path,
        method,
        parameters: parameters.map((parameter) => parameter.name),
      })),
    ).toEqual(operations.toSorted((a, b) => a.path.localeCompare(b.path)));
  });

  test("has every schema, and every type it names is one of them", () => {
    const names = new Set(reference.schemas.map((schema) => schema.name));
    expect([...names].toSorted()).toEqual(
      Object.keys(document.components?.schemas ?? {}).toSorted(),
    );
    const named = [
      ...reference.schemas.flatMap((schema) =>
        schema.fields.map((field) => field.type.schema),
      ),
      ...reference.endpoints.flatMap((item) =>
        item.responses.map((response) => response.type?.schema),
      ),
    ].filter((name) => name !== undefined);
    expect(named.length).toBeGreaterThan(0);
    for (const name of named) {
      expect(names).toContain(name);
    }
  });

  test("keeps the limits, the default and the values of a parameter", () => {
    const search = endpoint("search-stations");
    const radius = search.parameters.find((item) => item.name === "radius");
    expect(radius).toEqual({
      name: "radius",
      in: "query",
      required: false,
      description: "Around a point: the distance to search within, in metres.",
      type: { text: "number" },
      values: [],
      default: "1000",
      example: "300",
      limits: [
        { kind: "minimum", value: 1 },
        { kind: "maximum", value: 50000 },
      ],
    });
    const technology = search.parameters.find(
      (item) => item.name === "technology",
    );
    expect(technology?.type).toEqual({ text: "string[]" });
    expect(technology?.values).toEqual(["2G", "3G", "4G", "5G"]);
    expect(technology?.limits).toEqual([{ kind: "items", max: 4 }]);
    const id = endpoint("get-station").parameters.find(
      (item) => item.name === "id",
    );
    expect(id?.required).toBe(true);
    expect(id?.in).toBe("path");
    expect(id?.limits.map((rule) => rule.kind)).toEqual([
      "maxLength",
      "pattern",
    ]);
  });

  test("lists the responses of an endpoint, errors included", () => {
    expect(
      endpoint("get-station").responses.map(
        ({ status, contentType, type }) => ({
          status,
          contentType,
          schema: type?.schema,
        }),
      ),
    ).toEqual([
      {
        status: "200",
        contentType: "application/json",
        schema: "StationHistory",
      },
      { status: "304", contentType: undefined, schema: undefined },
      ...["404", "422", "500", "503"].map((status) => ({
        status,
        contentType: "application/problem+json",
        schema: "ErrorModel",
      })),
    ]);
    expect(endpoint("list-stations").responses[0]?.contentType).toBe(
      "application/geo+json",
    );
  });

  test("keeps the limits and the default of a field", () => {
    const field = (schema: string, name: string) =>
      reference.schemas
        .find((item) => item.name === schema)
        ?.fields.find((item) => item.name === name);
    expect(field("PointGeometry", "coordinates")?.limits).toEqual([
      { kind: "items", min: 2, max: 2 },
    ]);
    expect(field("Snapshot", "sha256")?.limits).toEqual([
      { kind: "pattern", value: "^[0-9a-f]{64}$" },
    ]);
    expect(field("ErrorModel", "type")?.default).toBe("about:blank");
  });

  test("describes caching once, from the contract", () => {
    expect(reference.caching.headers.map((header) => header.name)).toEqual([
      "Cache-Control",
      "ETag",
    ]);
    expect(reference.caching.request?.name).toBe("If-None-Match");
    expect(reference.caching.notModified?.status).toBe("304");
  });

  test("keeps the contract's description, version and licence", () => {
    expect(reference.version).toBe(document.info.version);
    expect(reference.description).toContain(
      "Source: Federal Office of Communications OFCOM",
    );
    expect(reference.license).toBe(document.info.license?.name);
  });
});

describe("the examples", () => {
  test("search around the point the contract gives", () => {
    expect(exampleCommand(origin, endpoint("search-stations"))).toBe(
      "curl -sS --compressed 'http://127.0.0.1:8080/v1/stations/search?lat=46.948&lon=7.4474&radius=300'",
    );
  });

  test("read a station by its identifier, percent-encoded", () => {
    expect(exampleUrl(origin, endpoint("get-station"))).toBe(
      "http://127.0.0.1:8080/v1/stations/2683328_1247916_Sunrise%20ZE416-1",
    );
  });

  test("give a path parameter for every endpoint that has one", () => {
    for (const item of reference.endpoints) {
      expect(exampleUrl(origin, item)).not.toContain("{");
    }
  });

  test("cannot end the quotes of the command", () => {
    const quoted: Endpoint = {
      ...endpoint("get-station"),
      parameters: [
        {
          name: "id",
          in: "path",
          required: true,
          description: "",
          type: { text: "string" },
          values: [],
          example: "1_2_O'Brien'; rm -rf ~",
          limits: [],
        },
      ],
    };
    expect(exampleCommand(origin, quoted)).toBe(
      "curl -sS --compressed 'http://127.0.0.1:8080/v1/stations/1_2_O%27Brien%27%3B%20rm%20-rf%20~'",
    );
  });

  test("refuse a path parameter without an example", () => {
    const missing: Endpoint = {
      ...endpoint("get-station"),
      parameters: [],
    };
    missing.parameters = [
      {
        name: "id",
        in: "path",
        required: true,
        description: "",
        type: { text: "string" },
        values: [],
        limits: [],
      },
    ];
    expect(() => exampleUrl(origin, missing)).toThrow(/no example for id/);
  });
});

describe("types and descriptions", () => {
  test("write lists, formats of strings, nullable values and references", () => {
    expect(typeName({ type: ["boolean", "null"] })).toEqual({
      text: "boolean | null",
    });
    expect(typeName({ type: "string", format: "date-time" })).toEqual({
      text: "string (date-time)",
    });
    expect(typeName({ type: "number", format: "double" })).toEqual({
      text: "number",
    });
    expect(
      typeName({
        type: "array",
        items: { $ref: "#/components/schemas/Period" },
      }),
    ).toEqual({ text: "Period[]", schema: "Period" });
    expect(typeName(undefined)).toEqual({ text: "any" });
  });

  test("write a reference to a schema that may be null as such", () => {
    const period = reference.schemas.find((schema) => schema.name === "Period");
    expect(
      period?.fields.find((field) => field.name === "permit")?.type,
    ).toEqual({ text: "Permit | null", schema: "Permit" });
    expect(period?.fields.find((field) => field.name === "to")?.type).toEqual({
      text: "string (date-time) | null",
    });
  });

  test("refuse a reference outside the contract's schemas", () => {
    expect(() =>
      readContract({
        info: { version: "1" },
        paths: {
          "/x": {
            get: {
              operationId: "x",
              responses: {
                "200": {
                  description: "OK",
                  content: {
                    "application/json": {
                      schema: { $ref: "https://example.org/schema" },
                    },
                  },
                },
              },
            },
          },
        },
      }),
    ).toThrow(/outside its schemas/);
  });

  test("show what is between backquotes as code, and the rest as text", () => {
    expect(segments("either `bbox`, or `lat` and <b>`lon`</b>")).toEqual([
      { code: false, text: "either " },
      { code: true, text: "bbox" },
      { code: false, text: ", or " },
      { code: true, text: "lat" },
      { code: false, text: " and <b>" },
      { code: true, text: "lon" },
      { code: false, text: "</b>" },
    ]);
  });
});
