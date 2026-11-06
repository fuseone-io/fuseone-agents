import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import {
  evidenceDeclarationValues,
  evidenceInstanceDefaults,
  evidenceInstancePayload,
  evidenceInstanceSchema,
  parseEvidenceDeclaration,
  type EvidenceInstanceValues,
} from "@/features/integrations/connectors/evidence-instance-model";
import { evidenceExamples } from "@/features/integrations/connectors/evidence-examples";

function manualDeclarations(locale: string) {
  const manual = readFileSync(
    `../docs/manual/${locale}/evidence-collection.md`,
    "utf8",
  );
  return [...manual.matchAll(/```json\n([\s\S]*?)```/g)].map((block) =>
    JSON.parse(block[1]!),
  );
}

const declarations = manualDeclarations("en-US");

function offsetValues(): EvidenceInstanceValues {
  return {
    ...evidenceInstanceDefaults(null, "acme", "cyber"),
    ...evidenceDeclarationValues(declarations[0]),
    name: "gateway",
    token: "read-only",
  };
}

function cursorValues(): EvidenceInstanceValues {
  return {
    ...evidenceInstanceDefaults(null, "acme", "cyber"),
    ...evidenceDeclarationValues(declarations[1]),
    name: "console",
    token: "read-only",
  };
}

function issuesOf(values: EvidenceInstanceValues) {
  const parsed = evidenceInstanceSchema.safeParse(values);
  return parsed.success ? [] : parsed.error.issues.map((i) => i.message);
}

describe("evidence examples", () => {
  // The Go suite holds the manual to the validator, so holding the examples to
  // the manual is what keeps an example from being refused on save.
  it.each(["en-US", "pt-BR"])(
    "are the declarations in the %s manual",
    (locale) => {
      expect(evidenceExamples.map((example) => example.declaration)).toEqual(
        manualDeclarations(locale),
      );
    },
  );
});

describe("evidence instance payload", () => {
  it.each([0, 1])("carries declaration %i whole through the form", (at) => {
    const values = {
      ...evidenceInstanceDefaults(null, "acme", "cyber"),
      ...evidenceDeclarationValues(declarations[at]),
      name: "source",
      token: "read-only",
    };

    const saved = evidenceInstancePayload(values, false);

    expect(saved?.body.evidence).toEqual(declarations[at]);
  });

  it("keeps the space that separates a token prefix from the token", () => {
    const saved = evidenceInstancePayload(cursorValues(), false);

    expect(saved?.body.evidence?.authPrefix).toBe("ApiToken ");
  });

  it("drops the settings of the paging and window kinds not in use", () => {
    const values = offsetValues();
    values.pagingKind = "cursor";
    values.cursorParam = "cursor";
    values.nextCursor = "pagination.nextCursor";
    values.windowKind = "range";
    values.startParam = "from";
    values.endParam = "to";
    values.format = "epoch";

    const saved = evidenceInstancePayload(values, false)?.body.evidence;

    expect(saved?.paging.offsetParam).toBeUndefined();
    expect(saved?.window.periodParam).toBeUndefined();
    expect(saved?.window.periods).toBeUndefined();
  });

  it("requires a token until one is stored", () => {
    const values = { ...offsetValues(), token: "" };

    expect(evidenceInstancePayload(values, false)).toBeNull();
    expect(evidenceInstancePayload(values, true)?.body.token).toBeUndefined();
  });

  it("refuses to clear the token of an enabled source without a new one", () => {
    const values = { ...offsetValues(), token: "", clearToken: true };

    expect(evidenceInstancePayload(values, true)).toBeNull();
  });

  it("clears the token of a disabled source", () => {
    const values = {
      ...offsetValues(),
      enabled: false,
      token: "",
      clearToken: true,
    };

    expect(evidenceInstancePayload(values, true)?.body.clearToken).toBe(true);
  });
});

describe("evidence instance schema", () => {
  it.each([0, 1])("accepts declaration %i", (at) => {
    const values = at === 0 ? offsetValues() : cursorValues();

    expect(issuesOf(values)).toEqual([]);
  });

  const breaks: [string, string, (values: EvidenceInstanceValues) => void][] = [
    [
      "a source that is not https",
      "connectors.evidenceBaseUrlInvalid",
      (v) => void (v.baseUrl = "http://tenant.example.com"),
    ],
    [
      "half a refusal",
      "connectors.evidenceMatchIncomplete",
      (v) => void (v.refusalValue = ""),
    ],
    [
      "half a gap",
      "connectors.evidenceMatchIncomplete",
      (v) => void (v.gapField = ""),
    ],
    [
      "descending periods",
      "connectors.evidencePeriodsInvalid",
      (v) => void (v.periods = "86400, 3600"),
    ],
    [
      "two queries with one id",
      "connectors.evidenceQueryDuplicate",
      (v) => void (v.queries[1]!.id = v.queries[0]!.id),
    ],
    [
      "a control no query answers to",
      "connectors.evidenceControlInvalid",
      (v) => void (v.control = "nothing"),
    ],
    [
      "a control with parameters",
      "connectors.evidenceControlInvalid",
      (v) => void (v.control = "pages_by_user"),
    ],
    [
      "a windowless control",
      "connectors.evidenceControlInvalid",
      (v) => void (v.queries[0]!.windowless = true),
    ],
    [
      "a gap field the query drops",
      "connectors.evidenceGapFieldDropped",
      (v) => void (v.queries[0]!.fields = "_id"),
    ],
    [
      "offset paging with no id field",
      "connectors.evidenceIdFieldRequired",
      (v) => void (v.queries[0]!.idField = ""),
    ],
    [
      "an id field the query drops",
      "connectors.evidenceIdFieldDropped",
      (v) => void (v.queries[0]!.idField = "uuid"),
    ],
    [
      "a time field the query drops",
      "connectors.evidenceTimeFieldDropped",
      (v) => void (v.queries[1]!.timeField = "when"),
    ],
    [
      "a filter that collides with paging",
      "connectors.evidenceFilterReserved",
      (v) => void (v.queries[1]!.filters = "limit = {user}"),
    ],
    [
      "a filter line with no separator",
      "connectors.evidenceLineInvalid",
      (v) => void (v.queries[1]!.filters = "query"),
    ],
    [
      "a placeholder no parameter declares",
      "connectors.evidencePlaceholderUndeclared",
      (v) => void (v.queries[1]!.filters = "query = user eq '{person}'"),
    ],
    [
      "a parameter no filter uses",
      "connectors.evidenceParameterUnused",
      (v) => void (v.queries[1]!.filters = "query = everything"),
    ],
    [
      "a path with a query string",
      "connectors.evidencePathInvalid",
      (v) => void (v.queries[0]!.path = "/api/events?type=page"),
    ],
  ];

  it.each(breaks)("refuses %s", (_, message, change) => {
    const values = offsetValues();
    change(values);

    expect(issuesOf(values)).toContain(message);
  });
});

function withQuery(change: Record<string, unknown>) {
  const [first, ...rest] = declarations[0].queries;
  return JSON.stringify({
    ...declarations[0],
    queries: [{ ...first, ...change }, ...rest],
  });
}

describe("parseEvidenceDeclaration", () => {
  it("reads a declaration from the manual", () => {
    const parsed = parseEvidenceDeclaration(JSON.stringify(declarations[0]));

    expect(parsed).toEqual(declarations[0]);
  });

  it.each([
    ["text that is not JSON", "{ nope"],
    ["a list", "[]"],
    ["an instance body rather than a declaration", '{"connector":"evidence"}'],
    [
      "a query with no fields",
      JSON.stringify({ ...declarations[0], queries: [{ id: "a" }] }),
    ],
    [
      "a misspelled filter list, which would widen the query",
      withQuery({ filterz: [{ name: "query", value: "x" }] }),
    ],
    [
      "an unknown key in a filter",
      withQuery({ filters: [{ name: "q", value: "x", extra: 1 }] }),
    ],
    [
      "an unknown key in a parameter",
      withQuery({ parameters: [{ name: "q", pattern: "x", extra: 1 }] }),
    ],
    ["filters that are not a list", withQuery({ filters: {} })],
    ["parameters that are not a list", withQuery({ parameters: "x" })],
    ["a filter that is null", withQuery({ filters: [null] })],
    ["fields that are not text", withQuery({ fields: [1] })],
    ["a windowless that is not a boolean", withQuery({ windowless: "yes" })],
    [
      "a query that is null",
      JSON.stringify({ ...declarations[0], queries: [null] }),
    ],
    [
      "an unknown key in paging",
      JSON.stringify({
        ...declarations[0],
        paging: { ...declarations[0].paging, pagesize: 1 },
      }),
    ],
    [
      "an unknown key in the window",
      JSON.stringify({
        ...declarations[0],
        window: { ...declarations[0].window, period: 1 },
      }),
    ],
    [
      "an unknown key in a refusal",
      JSON.stringify({
        ...declarations[0],
        refusal: { ...declarations[0].refusal, values: "0" },
      }),
    ],
    [
      "periods that are not numbers",
      JSON.stringify({
        ...declarations[0],
        window: { ...declarations[0].window, periods: ["3600"] },
      }),
    ],
    [
      "a paging kind that does not exist",
      JSON.stringify({ ...declarations[0], paging: { kind: "page" } }),
    ],
    [
      "a token beside the declaration",
      JSON.stringify({ ...declarations[0], token: "secret" }),
    ],
  ])("refuses %s", (_, text) => {
    expect(parseEvidenceDeclaration(text)).toBeNull();
  });
});
