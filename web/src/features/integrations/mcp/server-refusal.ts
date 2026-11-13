import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

/*
 * The server refuses a company's or an area's server outside the platform's
 * egress policy as invalid input, with no code of its own (NT-019), so the
 * sentence it ends in is what tells the refusal apart. The origin it names
 * after the colon is the particular, and is interpolated, not translated.
 */
const outsideEgress = "outside the platform's egress policy";

function egressOrigin(error: unknown): string | null {
  if (!(error instanceof ApiError) || error.problem?.type !== "fuseone:invalid-input") return null;
  const detail = error.detail ?? "";
  const at = detail.indexOf(outsideEgress);
  if (at < 0) return null;
  const rest = detail.slice(at + outsideEgress.length);
  const colon = rest.indexOf(": ");
  return colon < 0 ? "" : rest.slice(colon + 2).trim();
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
