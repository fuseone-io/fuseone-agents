import { z } from "zod";
import type { ConnectorEvidenceConfig } from "@/features/integrations/api";

// The shape of a pasted declaration, strict at every level. A key this does not
// know is refused rather than dropped: `filterz` for `filters` would otherwise
// vanish and leave a valid query with its filter gone, which reads more than
// the person meant. Whether the values make sense is the form's to judge, once
// they are in its fields.
const match = z.strictObject({
  field: z.string().optional(),
  value: z.string().optional(),
});

const paging = z.strictObject({
  kind: z.enum(["offset", "cursor"]),
  limitParam: z.string(),
  pageSize: z.number(),
  offsetParam: z.string().optional(),
  cursorParam: z.string().optional(),
  nextCursor: z.string().optional(),
});

const window = z.strictObject({
  kind: z.enum(["period", "range"]),
  periodParam: z.string().optional(),
  periods: z.array(z.number()).optional(),
  startParam: z.string().optional(),
  endParam: z.string().optional(),
  format: z.enum(["iso", "epoch"]).optional(),
});

const query = z.strictObject({
  id: z.string(),
  description: z.string(),
  path: z.string(),
  filters: z
    .array(z.strictObject({ name: z.string(), value: z.string() }))
    .optional(),
  parameters: z
    .array(z.strictObject({ name: z.string(), pattern: z.string() }))
    .optional(),
  fields: z.array(z.string()),
  idField: z.string().optional(),
  timeField: z.string().optional(),
  windowless: z.boolean().optional(),
});

const declaration = z.strictObject({
  baseUrl: z.string(),
  authHeader: z.string(),
  authPrefix: z.string().optional(),
  control: z.string(),
  rows: z.string(),
  refusal: match.optional(),
  gap: match.optional(),
  paging,
  window,
  maxEvents: z.number().optional(),
  queries: z.array(query),
}) satisfies z.ZodType<ConnectorEvidenceConfig>;

/** Reads a declaration someone pasted, or null when any part of it is off. */
export function parseEvidenceDeclaration(
  text: string,
): ConnectorEvidenceConfig | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  const result = declaration.safeParse(parsed);
  return result.success ? result.data : null;
}
