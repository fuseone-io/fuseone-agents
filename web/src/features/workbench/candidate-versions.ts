import type { CandidateSeal } from "@/lib/api/client";

/** One version as the list shows it: its newest seal, and how many times it was sealed. */
export type CandidateVersion = { seal: CandidateSeal; seals: number };

/**
 * The candidates grouped by version. Sealing the same text again gives the same version under
 * a new seal; the list names the version once. The seals arrive newest first, so the first of
 * each version is its newest, and the order of versions is kept.
 */
export function byVersion(seals: CandidateSeal[]): CandidateVersion[] {
  const out = new Map<string, CandidateVersion>();
  for (const seal of seals) {
    const seen = out.get(seal.version);
    if (seen) seen.seals += 1;
    else out.set(seal.version, { seal, seals: 1 });
  }
  return [...out.values()];
}
