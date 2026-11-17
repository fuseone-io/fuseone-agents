import type { AgentDefinition, DraftOperation } from "@/lib/api/client";

/**
 * The panel's edits, as the operations a draft takes (NT-018 1b, contract 1).
 *
 * The person edits a local copy with the same sections the editor uses; saving
 * sends, for each field they touched and that now differs, the one typed
 * operation that edits it. The instructions are never among them: they change
 * only through an accepted proposal of the conversation, which this panel does
 * not have.
 */

export type Field = keyof AgentDefinition;
type Def = AgentDefinition;
type Step = NonNullable<Def["steps"]>[number];

/** Compared by value: what the wire would carry, absent and empty alike. */
function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
}

function changed(before: Def, after: Def, touched: ReadonlySet<Field>, fields: Field[]) {
  return fields.some((f) => touched.has(f) && !same(before[f], after[f]));
}

/** The operation each whole field is set by, for fields set whole. */
const WHOLE: { fields: Field[]; op: (d: Def) => DraftOperation }[] = [
  {
    fields: ["name", "company", "area"],
    op: (d) => ({ kind: "set_identity", name: d.name, company: d.company, area: d.area }),
  },
  {
    fields: ["standingApprovals"],
    op: (d) => ({ kind: "set_standing_approvals", standingApprovals: d.standingApprovals }),
  },
  {
    fields: ["provider", "model", "effort"],
    op: (d) => ({ kind: "set_model", provider: d.provider, model: d.model, effort: d.effort }),
  },
  { fields: ["fallback"], op: (d) => ({ kind: "set_fallback", fallback: d.fallback }) },
  {
    fields: ["cachePrefixTtl"],
    op: (d) => ({ kind: "set_cache_prefix_ttl", cachePrefixTtl: d.cachePrefixTtl }),
  },
  { fields: ["replyLocale"], op: (d) => ({ kind: "set_reply_locale", replyLocale: d.replyLocale }) },
  { fields: ["budget"], op: (d) => ({ kind: "set_budget", budget: d.budget }) },
  { fields: ["triggers"], op: (d) => ({ kind: "set_triggers", triggers: d.triggers ?? [] }) },
  {
    fields: ["memoryLearning"],
    op: (d) => ({ kind: "set_memory_learning", memoryLearning: d.memoryLearning }),
  },
  { fields: ["approvals"], op: (d) => ({ kind: "set_approval_delivery", approvals: d.approvals }) },
  { fields: ["emits"], op: (d) => ({ kind: "set_emits", emits: d.emits ?? [] }) },
];

/** Tools added, then tools removed: a step never loses a tool midway. */
function toolOperations(before: Def, after: Def): DraftOperation[] {
  const was = before.tools ?? [];
  const now = after.tools ?? [];
  return [
    ...now.filter((t) => !was.includes(t)).map((tool) => ({ kind: "add_tool" as const, tool })),
    ...was.filter((t) => !now.includes(t)).map((tool) => ({ kind: "remove_tool" as const, tool })),
  ];
}

/** Steps by name: edited and added in their new order, then removed. */
function stepOperations(before: Def, after: Def): DraftOperation[] {
  const was = new Map<string, Step>((before.steps ?? []).map((s) => [s.name, s]));
  const now = after.steps ?? [];
  const out: DraftOperation[] = [];
  for (const step of now) {
    const old = was.get(step.name);
    if (!old) out.push({ kind: "add_step", step });
    else if (!same(old, step)) out.push({ kind: "edit_step", stepName: step.name, step });
  }
  const kept = new Set(now.map((s) => s.name));
  for (const name of was.keys()) {
    if (!kept.has(name)) out.push({ kind: "remove_step", stepName: name });
  }
  return out;
}

export function operationsFor(before: Def, after: Def, touched: ReadonlySet<Field>): DraftOperation[] {
  const out: DraftOperation[] = [];
  for (const { fields, op } of WHOLE) {
    if (changed(before, after, touched, fields)) out.push(op(after));
  }
  if (changed(before, after, touched, ["tools"])) out.push(...toolOperations(before, after));
  if (changed(before, after, touched, ["steps"])) out.push(...stepOperations(before, after));
  return out;
}

/**
 * The person's edits over a draft read again after somebody else wrote: the
 * newer draft, with every field the person touched put back as they left it.
 * Nothing they typed is lost to a 412.
 */
export function rebase(server: Def, local: Def, touched: ReadonlySet<Field>): Def {
  const out: Def = { ...server };
  for (const field of touched) {
    if (field === "instructions") continue;
    (out as Record<Field, unknown>)[field] = local[field];
  }
  return out;
}
