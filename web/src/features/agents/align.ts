export interface Step {
  kind: "same" | "added" | "removed";
  /** Where it sits in the earlier sequence. Absent on an addition. */
  ai?: number;
  /** Where it sits in the later one. Absent on a removal. */
  bi?: number;
}

/**
 * Two sequences, lined up by their longest common subsequence.
 *
 * The ordinary diff, and the reason it is worth the table rather than a
 * position-by-position comparison: one item inserted at the front shifts
 * everything after it, and a diff that reports the shift reports that
 * everything changed.
 *
 * Used on blocks and then on the words inside one, which is why it takes
 * strings and nothing else — the caller decides what makes two items the same
 * by choosing what to turn them into.
 */
export function align(a: string[], b: string[]): Step[] {
  const common = table(a, b);

  const steps: Step[] = [];
  let i = 0;
  let j = 0;

  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      steps.push({ kind: "same", ai: i, bi: j });
      i++;
      j++;
    } else if (common[i + 1]![j]! >= common[i]![j + 1]!) {
      steps.push({ kind: "removed", ai: i++ });
    } else {
      steps.push({ kind: "added", bi: j++ });
    }
  }
  while (i < a.length) steps.push({ kind: "removed", ai: i++ });
  while (j < b.length) steps.push({ kind: "added", bi: j++ });

  return steps;
}

/** Lengths of the longest common subsequence of every pair of suffixes. */
function table(a: string[], b: string[]): number[][] {
  const rows = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0),
  );

  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      rows[i]![j] =
        a[i] === b[j]
          ? rows[i + 1]![j + 1]! + 1
          : Math.max(rows[i + 1]![j]!, rows[i]![j + 1]!);
    }
  }
  return rows;
}
