import { z } from "zod";
import type { PolicyInput } from "@/lib/api/client";

/**
 * A reach that names nobody governs nothing, and the server refuses it. Saying
 * so at the button keeps the refusal from arriving minutes later as a toast.
 */
export function reachNamesNobody(draft: PolicyInput): boolean {
  if (draft.reach === "agents") return (draft.agents ?? []).length === 0;
  if (draft.reach === "scopes") return (draft.scopes ?? []).length === 0;
  return false;
}

const conditionSchema = z.object({
  field: z.string(),
  op: z.enum(["eq", "ne", "gt", "lt", "contains", "in"]),
  value: z.string(),
});

/**
 * The policy draft, and the single place that says whether it can be saved.
 *
 * The code is not here: it is the record's identity, owned by `useCode` and
 * sent beside the policy rather than inside it.
 */
export const policyDraftSchema = z
  .object({
    name: z.string().min(1),
    owner: z.string().optional(),
    reason: z.string().optional(),
    resource: z.string().optional(),
    effects: z
      .array(z.enum(["read", "write", "destructive", "financial"]))
      .optional(),
    reach: z.enum(["installation", "scopes", "agents"]).optional(),
    scopes: z
      .array(z.object({ company: z.string(), area: z.string() }))
      .optional(),
    agents: z.array(z.string()).optional(),
    conditions: z.array(conditionSchema).optional(),
    effect: z.enum(["allow", "escalate", "deny"]),
    mode: z.enum(["monitor", "enforce"]),
    enabled: z.boolean().optional(),
  })
  .refine((draft) => !reachNamesNobody(draft), {
    path: ["reach"],
  }) satisfies z.ZodType<PolicyInput>;
