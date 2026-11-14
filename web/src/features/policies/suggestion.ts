/** One value a field can offer, with a hint shown beside it. */
export type Suggestion = { value: string; hint?: string };

export function dedupe(suggestions: Suggestion[]): Suggestion[] {
  const seen = new Set<string>();
  return suggestions.filter((s) => {
    if (s.value === "" || seen.has(s.value)) return false;
    seen.add(s.value);
    return true;
  });
}
