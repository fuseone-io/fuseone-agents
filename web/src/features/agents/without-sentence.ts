/**
 * The sentence that names a tool, taken out and nothing else.
 *
 * Sentence-wise rather than the whole block: an author who wrote four
 * sentences and is being told about one should not lose the other three, and
 * "remove the sentence" has to mean what it says.
 */
export function withoutSentence(text: string, tool: string): string {
  return text
    .split(/(?<=[.!?])\s+/)
    .filter((sentence) => !sentence.includes(tool))
    .join(" ")
    .trim();
}
