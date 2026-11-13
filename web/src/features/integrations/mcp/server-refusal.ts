import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

/*
 * A company's or an area's server outside the platform's egress policy for
 * its scope (NT-019). The problem's type names the refusal and its detail is
 * the canonical origin alone, interpolated rather than translated.
 */
function egressOrigin(error: unknown): string | null {
  if (!(error instanceof ApiError) || error.problem?.type !== "fuseone:egress-not-allowed") return null;
  return error.detail ?? "";
}

/** A refusal said beside the field it is about, or null for any other. */
export function serverRefusal(
  error: unknown,
  t: TFunction,
): { field: "name" | "url"; message: string } | null {
  if (error instanceof ApiError && error.problem?.type === "fuseone:integration-name-taken") {
    return { field: "name", message: problemMessage(error, t) };
  }
  const origin = egressOrigin(error);
  if (origin === null) return null;
  return { field: "url", message: egressMessage(origin, t) };
}

/** What to say about a server write refused anywhere a field cannot carry it. */
export function serverProblemMessage(error: unknown, t: TFunction) {
  const origin = egressOrigin(error);
  return origin === null ? problemMessage(error, t) : egressMessage(origin, t);
}

function egressMessage(origin: string, t: TFunction) {
  return origin
    ? t("integrations.outsideEgressAt", { origin, interpolation: { escapeValue: false } })
    : t("integrations.outsideEgress");
}
