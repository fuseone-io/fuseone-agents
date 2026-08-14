import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";

/**
 * What to say to a person about a refusal.
 *
 * The server names the condition and this chooses the words. It used to show
 * whatever sentence the server held, which was Portuguese for half the
 * refusals and English for the other half, in whichever language the reader
 * had chosen — and a client that was not this console had nothing to branch on
 * but prose.
 *
 * The particulars stay the server's: an identifier, a permission, a scope.
 * They are interpolated rather than parsed, so the sentence around them is
 * this console's to write and to translate.
 */
export function problemMessage(error: unknown, t: TFunction): string {
  // Not every failure reaches here as a refusal: the network dropping, or a
  // reply that was never JSON, arrives as an ordinary Error. There is nothing
  // to interpolate and nothing to name, so it gets the one sentence that is
  // true of all of them.
  if (!(error instanceof ApiError)) {
    return t("common.requestFailed");
  }

  const code = error.problem?.type;
  if (code && code.startsWith("fuseone:")) {
    return t(`problem.${code.slice("fuseone:".length)}`, {
      detail: error.detail ?? "",
      // A code this console does not know is still a refusal somebody has to
      // read. The server's title is English and terse, which beats a blank.
      defaultValue: error.problem?.title ?? t("common.requestFailed"),
    });
  }
  return error.problem?.title ?? t("common.requestFailed");
}
