import contract from "../../../openapi.json";

// The API's documentation, read from its contract: the page about the API
// renders what this module returns, so that it says what openapi.json says
// and nothing else (decision 28 in docs/decisions.md). The contract is the
// one the website is built with, the same one its types come from.

// The parts of an OpenAPI 3.1 document that the page shows.
interface SchemaObject {
  $ref?: string;
  type?: string | string[];
  format?: string;
  description?: string;
  enum?: unknown[];
  items?: SchemaObject;
  properties?: Record<string, SchemaObject>;
  required?: string[];
  default?: unknown;
  examples?: unknown[];
  minimum?: number;
  maximum?: number;
  minItems?: number;
  maxItems?: number;
  maxLength?: number;
  pattern?: string;
}

interface ParameterObject {
  name: string;
  in: string;
  description?: string;
  required?: boolean;
  schema?: SchemaObject;
}

interface ResponseObject {
  description: string;
  content?: Record<string, { schema?: SchemaObject }>;
  headers?: Record<string, { description?: string }>;
}

interface OperationObject {
  operationId: string;
  summary?: string;
  description?: string;
  tags?: string[];
  parameters?: ParameterObject[];
  responses: Record<string, ResponseObject>;
}

export interface OpenApiDocument {
  info: {
    title: string;
    version: string;
    description?: string;
    license?: { name: string; identifier?: string };
  };
  paths: Record<string, Record<string, OperationObject>>;
  components?: { schemas?: Record<string, SchemaObject> };
}

// A type as the page writes it: the name of a schema, which links to it, or
// a type of JSON, with the format, `[]` for a list, `| null` when it may be
// null.
export interface TypeName {
  text: string;
  // The schema it names, if it names one.
  schema?: string;
}

// What restricts a value, each as one rule the page translates.
export type Limit =
  | { kind: "minimum" | "maximum"; value: number }
  | { kind: "items"; min?: number; max?: number }
  | { kind: "maxLength"; value: number }
  | { kind: "pattern"; value: string };

export interface Parameter {
  name: string;
  in: string;
  required: boolean;
  description: string;
  type: TypeName;
  values: string[];
  default?: string;
  example?: string;
  limits: Limit[];
}

export interface Response {
  status: string;
  description: string;
  contentType?: string;
  type?: TypeName;
}

export interface Endpoint {
  id: string;
  method: string;
  path: string;
  summary: string;
  description: string;
  tag: string;
  parameters: Parameter[];
  responses: Response[];
}

export interface Field {
  name: string;
  required: boolean;
  description: string;
  type: TypeName;
  values: string[];
}

export interface Schema {
  name: string;
  description: string;
  fields: Field[];
}

// What every response of the API says about caching, written once in the
// contract for each operation and once on the page.
export interface Caching {
  // The headers of a successful response.
  headers: { name: string; description: string }[];
  // The header a client sends back.
  request?: { name: string; description: string };
  notModified?: { status: string; description: string };
}

export interface Reference {
  title: string;
  version: string;
  description: string;
  license?: string;
  endpoints: Endpoint[];
  caching: Caching;
  schemas: Schema[];
}

const refPrefix = "#/components/schemas/";

function schemaName(ref: string): string {
  if (!ref.startsWith(refPrefix)) {
    throw new Error(`The contract refers to ${ref}, outside its schemas`);
  }
  return ref.slice(refPrefix.length);
}

function text(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value);
}

// Whether a schema's type allows null.
function nullable(schema: SchemaObject | undefined): boolean {
  return Array.isArray(schema?.type) && schema.type.includes("null");
}

// `schemas` are the contract's own: a reference to one that may be null, such
// as a permit, is written so.
export function typeName(
  schema: SchemaObject | undefined,
  schemas: Record<string, SchemaObject> = {},
): TypeName {
  if (schema === undefined) {
    return { text: "any" };
  }
  if (schema.$ref !== undefined) {
    const name = schemaName(schema.$ref);
    return {
      text: nullable(schemas[name]) ? `${name} | null` : name,
      schema: name,
    };
  }
  const types = Array.isArray(schema.type)
    ? schema.type
    : schema.type === undefined
      ? []
      : [schema.type];
  const base = types.find((type) => type !== "null");
  let result: TypeName;
  if (base === "array") {
    const items = typeName(schema.items, schemas);
    result = { ...items, text: `${items.text}[]` };
  } else if (base === undefined) {
    result = { text: "any" };
  } else {
    // The formats of numbers are how Go holds them, not what a client
    // reads: only those of strings, such as a date, are worth showing.
    const format =
      base === "string" && schema.format !== undefined
        ? ` (${schema.format})`
        : "";
    result = { text: `${base}${format}` };
  }
  return nullable(schema)
    ? { ...result, text: `${result.text} | null` }
    : result;
}

// The values a schema allows, when it lists them, as for an enumeration or a
// list of one.
function values(schema: SchemaObject | undefined): string[] {
  const list = schema?.enum ?? schema?.items?.enum ?? [];
  return list.map(text);
}

function limits(schema: SchemaObject | undefined): Limit[] {
  const result: Limit[] = [];
  if (schema === undefined) {
    return result;
  }
  if (schema.minimum !== undefined) {
    result.push({ kind: "minimum", value: schema.minimum });
  }
  if (schema.maximum !== undefined) {
    result.push({ kind: "maximum", value: schema.maximum });
  }
  if (schema.minItems !== undefined || schema.maxItems !== undefined) {
    result.push({
      kind: "items",
      ...(schema.minItems === undefined ? {} : { min: schema.minItems }),
      ...(schema.maxItems === undefined ? {} : { max: schema.maxItems }),
    });
  }
  if (schema.maxLength !== undefined) {
    result.push({ kind: "maxLength", value: schema.maxLength });
  }
  if (schema.pattern !== undefined) {
    result.push({ kind: "pattern", value: schema.pattern });
  }
  return result;
}

// The description of a value, or of what it refers to when it has none.
function describe(
  schema: SchemaObject | undefined,
  description: string | undefined,
): string {
  return description ?? schema?.description ?? schema?.items?.description ?? "";
}

const cachingHeader = "If-None-Match";

function parameterOf(
  parameter: ParameterObject,
  schemas: Record<string, SchemaObject>,
): Parameter {
  const schema = parameter.schema;
  return {
    name: parameter.name,
    in: parameter.in,
    required: parameter.required ?? false,
    description: describe(schema, parameter.description),
    type: typeName(schema, schemas),
    values: values(schema),
    ...(schema?.default === undefined ? {} : { default: text(schema.default) }),
    ...(schema?.examples?.[0] === undefined
      ? {}
      : { example: text(schema.examples[0]) }),
    limits: limits(schema),
  };
}

function responsesOf(
  operation: OperationObject,
  schemas: Record<string, SchemaObject>,
): Response[] {
  return Object.entries(operation.responses)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([status, response]) => {
      const [contentType, media] =
        Object.entries(response.content ?? {})[0] ?? [];
      return {
        status,
        description: response.description,
        ...(contentType === undefined ? {} : { contentType }),
        ...(media?.schema === undefined
          ? {}
          : { type: typeName(media.schema, schemas) }),
      };
    });
}

// The fields of a schema: those it requires first, then the others, each in
// the order of the names, as the contract writes them.
function schemaOf(
  name: string,
  schema: SchemaObject,
  schemas: Record<string, SchemaObject>,
): Schema {
  const required = new Set(schema.required ?? []);
  const fields = Object.entries(schema.properties ?? {}).map(
    ([field, value]): Field => ({
      name: field,
      required: required.has(field),
      description: describe(value, value.description),
      type: typeName(value, schemas),
      values: values(value),
    }),
  );
  return {
    name,
    description: schema.description ?? "",
    fields: [
      ...fields.filter((field) => field.required),
      ...fields.filter((field) => !field.required),
    ],
  };
}

export function readContract(document: OpenApiDocument): Reference {
  const all = document.components?.schemas ?? {};
  const endpoints: Endpoint[] = [];
  let caching: Caching = { headers: [] };
  for (const [path, operations] of Object.entries(document.paths)) {
    for (const [method, operation] of Object.entries(operations)) {
      const parameters = operation.parameters ?? [];
      const ok = operation.responses["200"];
      const notModified = operation.responses["304"];
      const request = parameters.find(
        (parameter) =>
          parameter.in === "header" && parameter.name === cachingHeader,
      );
      if (caching.headers.length === 0 && ok?.headers !== undefined) {
        caching = {
          headers: Object.entries(ok.headers)
            .map(([name, header]) => ({
              name,
              description: header.description ?? "",
            }))
            .sort((a, b) => a.name.localeCompare(b.name)),
          ...(request === undefined
            ? {}
            : {
                request: {
                  name: request.name,
                  description: describe(request.schema, request.description),
                },
              }),
          ...(notModified === undefined
            ? {}
            : {
                notModified: {
                  status: "304",
                  description: notModified.description,
                },
              }),
        };
      }
      endpoints.push({
        id: operation.operationId,
        method: method.toUpperCase(),
        path,
        summary: operation.summary ?? "",
        description: operation.description ?? "",
        tag: operation.tags?.[0] ?? "",
        // The header of caching is the same for every operation: the page
        // describes it once.
        parameters: parameters
          .filter((parameter) => parameter !== request)
          .map((parameter) => parameterOf(parameter, all)),
        responses: responsesOf(operation, all),
      });
    }
  }
  endpoints.sort((a, b) => a.path.localeCompare(b.path));
  const schemas = Object.entries(all)
    .map(([name, schema]) => schemaOf(name, schema, all))
    .sort((a, b) => a.name.localeCompare(b.name));
  const license = document.info.license;
  return {
    title: document.info.title,
    version: document.info.version,
    description: document.info.description ?? "",
    ...(license === undefined ? {} : { license: license.name }),
    endpoints,
    caching,
    schemas,
  };
}

// The address of a request built from the examples the contract gives: each
// path parameter replaced by its example, percent-encoded, and each query
// parameter that has one added to the query. A parameter without an example
// is left out, so an example in the contract is a promise that the request it
// makes works.
export function exampleUrl(origin: string, endpoint: Endpoint): string {
  let path = endpoint.path;
  const query: string[] = [];
  for (const parameter of endpoint.parameters) {
    const value = parameter.example ?? parameter.default;
    if (parameter.in === "path") {
      if (value === undefined) {
        throw new Error(
          `${endpoint.path} has no example for ${parameter.name}`,
        );
      }
      path = path.replace(`{${parameter.name}}`, encodeURIComponent(value));
    } else if (parameter.in === "query" && parameter.example !== undefined) {
      query.push(
        `${encodeURIComponent(parameter.name)}=${encodeURIComponent(parameter.example)}`,
      );
    }
  }
  return `${origin}${path}${query.length === 0 ? "" : `?${query.join("&")}`}`;
}

// The command that sends that request, to copy into a shell. The address is
// quoted once; a quote in it, which percent-encoding leaves as it is, is
// encoded too so that it cannot end the quotes.
export function exampleCommand(origin: string, endpoint: Endpoint): string {
  const url = exampleUrl(origin, endpoint).replaceAll("'", "%27");
  return `curl -sS --compressed '${url}'`;
}

// A description as the page shows it: text, with what the contract writes
// between backquotes as code. Nothing in it is ever read as HTML.
export function segments(
  description: string,
): { code: boolean; text: string }[] {
  return description
    .split("`")
    .map((part, index) => ({ code: index % 2 === 1, text: part }))
    .filter((part) => part.text !== "");
}

export const reference: Reference = readContract(
  contract as unknown as OpenApiDocument,
);
