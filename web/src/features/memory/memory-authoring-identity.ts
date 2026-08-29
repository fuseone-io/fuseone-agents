export const HUMAN_MEMORY_KIND = "fact";

/**
 * The identity the server derives for a new fact taught by a person.
 *
 * This deliberately stays boring. A model turn here would add cost and make
 * the same subject produce different identities; the canonical identity on
 * the server already folds Unicode, case and whitespace before matching.
 */
export function derivedMemoryIdentity(subject: string | undefined) {
  return {
    kind: HUMAN_MEMORY_KIND,
    signature: subject?.trim() ?? "",
  };
}
