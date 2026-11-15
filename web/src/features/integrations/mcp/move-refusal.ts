import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

export type StrandedAgent = { agentId: string; company: string; area: string };

/**
 * What a refused move says (NT-019): the agents it would strand, to list and
 * decide on; an origin the target's policy does not admit, named; or the
 * shared message every other refusal has.
 */
export function moveRefusal(
  problem: unknown,
  t: TFunction,
  target: string,
): { stranded: StrandedAgent[] } | { message: string } {
  if (problem instanceof ApiError) {
    const body = problem.problem as { type?: string; detail?: string; agents?: StrandedAgent[] } | undefined;
    if (body?.type === "fuseone:move-strands-agents") return { stranded: body.agents ?? [] };
    if (body?.type === "fuseone:egress-not-allowed") {
      return { message: t("mcp.move.egressRefused", { scope: target, origin: body.detail ?? "" }) };
    }
    if (body?.type === "fuseone:server-not-at-installation") return { message: t("mcp.move.notAtInstallation") };
    if (problem.status === 412) return { message: t("mcp.move.stale") };
  }
  return { message: problemMessage(problem, t) };
}
