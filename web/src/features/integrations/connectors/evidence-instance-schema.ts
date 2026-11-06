import { z } from "zod";
import { connectorInstanceName } from "@/features/integrations/connectors/connector-instance-model";

// The same rules as connectortools/evidence_validate.go. The server stays the
// authority: these exist so a person hears about a problem on the field that
// has it. A parameter's pattern is not compiled here, because the browser's
// regular expressions are not the server's and would refuse valid ones.
const queryId = /^[a-z0-9][a-z0-9_-]{0,62}$/;
const header = /^[A-Za-z0-9-]{1,64}$/;
const param = /^[A-Za-z0-9_.[\]-]{1,64}$/;
const path = /^(\/[A-Za-z0-9_.~-]+)+$/;
const fieldName = /^[A-Za-z0-9_@][A-Za-z0-9@._-]{0,254}$/;

const querySchema = z.object({
  id: z.string().regex(queryId, "connectors.evidenceQueryIdInvalid"),
  description: z
    .string()
    .trim()
    .min(1, "connectors.evidenceDescriptionRequired"),
  path: z
    .string()
    .regex(path, "connectors.evidencePathInvalid")
    .refine((value) => !value.includes(".."), "connectors.evidencePathInvalid"),
  filters: z.string(),
  parameters: z.string(),
  fields: z.string(),
  idField: z.string(),
  timeField: z.string(),
  windowless: z.boolean(),
});

export const evidenceInstanceSchema = z
  .object({
    name: z
      .string()
      .regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    baseUrl: z
      .string()
      .regex(/^https:\/\/[^\s/]+/, "connectors.evidenceBaseUrlInvalid"),
    authHeader: z.string().regex(header, "connectors.evidenceHeaderInvalid"),
    // Never trimmed: "ApiToken " carries the space that separates it from the
    // token, and a tidied prefix is a source that refuses every request.
    authPrefix: z.string(),
    control: z.string(),
    rows: z.string().trim().min(1, "connectors.evidenceRowsRequired"),
    refusalField: z.string(),
    refusalValue: z.string(),
    gapField: z.string(),
    gapValue: z.string(),
    maxEvents: z.coerce
      .number()
      .int("connectors.evidenceMaxEventsInvalid")
      .min(0, "connectors.evidenceMaxEventsInvalid")
      .max(1_000_000, "connectors.evidenceMaxEventsInvalid"),
    pagingKind: z.enum(["offset", "cursor"]),
    limitParam: z.string().regex(param, "connectors.evidenceParamInvalid"),
    pageSize: z.coerce
      .number()
      .int("connectors.evidencePageSizeInvalid")
      .min(1, "connectors.evidencePageSizeInvalid")
      .max(10_000, "connectors.evidencePageSizeInvalid"),
    offsetParam: z.string(),
    cursorParam: z.string(),
    nextCursor: z.string(),
    windowKind: z.enum(["period", "range"]),
    periodParam: z.string(),
    periods: z.string(),
    startParam: z.string(),
    endParam: z.string(),
    format: z.enum(["iso", "epoch"]),
    queries: z.array(querySchema).min(1, "connectors.evidenceQueryRequired"),
    token: z.string(),
    clearToken: z.boolean(),
  })
  .superRefine((values, ctx) => {
    requireScope(values, ctx);
    requirePairs(values, ctx);
    requirePaging(values, ctx);
    requireWindow(values, ctx);
    requireQueries(values, ctx);
    requireControl(values, ctx);
  });

export type EvidenceInstanceValues = z.infer<typeof evidenceInstanceSchema>;
export type EvidenceQueryValues = EvidenceInstanceValues["queries"][number];

type Ctx = z.RefinementCtx;

/** Fields are separated by a new line or a comma. */
export function evidenceList(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

/** A line of `name = value`, split on the first `=`. Null when one has none. */
export function evidencePairs(text: string): [string, string][] | null {
  const pairs: [string, string][] = [];
  for (const line of text.split("\n")) {
    if (line.trim() === "") continue;
    const at = line.indexOf("=");
    if (at < 1) return null;
    pairs.push([line.slice(0, at).trim(), line.slice(at + 1).trim()]);
  }
  return pairs;
}

export function evidenceControlEligible(query: EvidenceQueryValues) {
  return query.id !== "" && !query.windowless && query.parameters.trim() === "";
}

function issue(ctx: Ctx, at: (string | number)[], message: string) {
  ctx.addIssue({ code: "custom", path: at, message });
}

function requireScope(values: EvidenceInstanceValues, ctx: Ctx) {
  if (values.scopeKind !== "installation" && values.company.trim() === "") {
    issue(ctx, ["company"], "connectors.companyRequired");
  }
  if (values.scopeKind === "area" && values.area.trim() === "") {
    issue(ctx, ["area"], "connectors.areaRequired");
  }
}

function requirePairs(values: EvidenceInstanceValues, ctx: Ctx) {
  if ((values.refusalField === "") !== (values.refusalValue === "")) {
    const at = values.refusalField === "" ? "refusalField" : "refusalValue";
    issue(ctx, [at], "connectors.evidenceMatchIncomplete");
  }
  if ((values.gapField === "") !== (values.gapValue === "")) {
    const at = values.gapField === "" ? "gapField" : "gapValue";
    issue(ctx, [at], "connectors.evidenceMatchIncomplete");
  }
}

function requirePaging(values: EvidenceInstanceValues, ctx: Ctx) {
  if (values.pagingKind === "offset") {
    if (!param.test(values.offsetParam)) {
      issue(ctx, ["offsetParam"], "connectors.evidenceParamInvalid");
    }
    return;
  }
  if (!param.test(values.cursorParam)) {
    issue(ctx, ["cursorParam"], "connectors.evidenceParamInvalid");
  }
  if (values.nextCursor.trim() === "") {
    issue(ctx, ["nextCursor"], "connectors.evidenceNextCursorRequired");
  }
}

function requireWindow(values: EvidenceInstanceValues, ctx: Ctx) {
  if (values.windowKind === "range") {
    for (const at of ["startParam", "endParam"] as const) {
      if (!param.test(values[at])) {
        issue(ctx, [at], "connectors.evidenceParamInvalid");
      }
    }
    return;
  }
  if (!param.test(values.periodParam)) {
    issue(ctx, ["periodParam"], "connectors.evidenceParamInvalid");
  }
  const periods = evidenceList(values.periods).map(Number);
  const ascending = periods.every(
    (period, at) =>
      Number.isInteger(period) &&
      period >= 1 &&
      (at === 0 || period >= periods[at - 1]!),
  );
  if (periods.length === 0 || !ascending) {
    issue(ctx, ["periods"], "connectors.evidencePeriodsInvalid");
  }
}

function reservedParams(values: EvidenceInstanceValues) {
  const paging =
    values.pagingKind === "offset"
      ? [values.offsetParam]
      : [values.cursorParam];
  const window =
    values.windowKind === "period"
      ? [values.periodParam]
      : [values.startParam, values.endParam];
  return new Set([values.limitParam, ...paging, ...window]);
}

function requireQueries(values: EvidenceInstanceValues, ctx: Ctx) {
  const seen = new Set<string>();
  values.queries.forEach((query, at) => {
    if (seen.has(query.id)) {
      issue(ctx, ["queries", at, "id"], "connectors.evidenceQueryDuplicate");
    }
    seen.add(query.id);
    requireFields(values, query, at, ctx);
    requireFilters(values, query, at, ctx);
  });
}

function requireFields(
  values: EvidenceInstanceValues,
  query: EvidenceQueryValues,
  at: number,
  ctx: Ctx,
) {
  const fields = evidenceList(query.fields);
  const where = ["queries", at];
  if (fields.length === 0 || !fields.every((f) => fieldName.test(f))) {
    issue(ctx, [...where, "fields"], "connectors.evidenceFieldsInvalid");
  }
  if (query.idField !== "" && !fields.includes(query.idField)) {
    issue(ctx, [...where, "idField"], "connectors.evidenceIdFieldDropped");
  }
  if (query.timeField !== "" && !fields.includes(query.timeField)) {
    issue(ctx, [...where, "timeField"], "connectors.evidenceTimeFieldDropped");
  }
  if (values.gapField !== "" && !fields.includes(values.gapField)) {
    issue(ctx, [...where, "fields"], "connectors.evidenceGapFieldDropped");
  }
  if (values.pagingKind === "offset" && query.idField === "") {
    issue(ctx, [...where, "idField"], "connectors.evidenceIdFieldRequired");
  }
}

function requireFilters(
  values: EvidenceInstanceValues,
  query: EvidenceQueryValues,
  at: number,
  ctx: Ctx,
) {
  const filtersAt = ["queries", at, "filters"];
  const parametersAt = ["queries", at, "parameters"];
  const filters = evidencePairs(query.filters);
  const parameters = evidencePairs(query.parameters);
  if (!filters) issue(ctx, filtersAt, "connectors.evidenceLineInvalid");
  if (!parameters) issue(ctx, parametersAt, "connectors.evidenceLineInvalid");
  if (!filters || !parameters) return;

  const reserved = reservedParams(values);
  const used = new Set<string>();
  for (const [name, value] of filters) {
    if (!param.test(name) || reserved.has(name)) {
      issue(ctx, filtersAt, "connectors.evidenceFilterReserved");
    }
    for (const match of value.matchAll(/\{([^{}]*)\}/g)) used.add(match[1]!);
  }
  const declared = new Set(parameters.map(([name]) => name));
  if ([...used].some((name) => !declared.has(name))) {
    issue(ctx, filtersAt, "connectors.evidencePlaceholderUndeclared");
  }
  for (const [name, pattern] of parameters) {
    if (!queryId.test(name) || !used.has(name)) {
      issue(ctx, parametersAt, "connectors.evidenceParameterUnused");
    }
    if (pattern === "") {
      issue(ctx, parametersAt, "connectors.evidencePatternRequired");
    }
  }
}

function requireControl(values: EvidenceInstanceValues, ctx: Ctx) {
  const control = values.queries.find((query) => query.id === values.control);
  if (!control || !evidenceControlEligible(control)) {
    issue(ctx, ["control"], "connectors.evidenceControlInvalid");
  }
}
