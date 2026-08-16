/**
 * `NAME=value` a line at a time, which is what an operator already has in
 * front of them — usually pasted out of a compose file or a shell export.
 *
 * Split on the first `=` only. A value is very often a token with an `=` in
 * it, and a credential silently cut in half fails somewhere nobody will
 * connect back to this box. A line without one is dropped rather than guessed
 * at, for the same reason.
 */
export function readVariables(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (trimmed === "" || trimmed.startsWith("#")) continue;
    const at = trimmed.indexOf("=");
    if (at <= 0) continue;
    out[trimmed.slice(0, at).trim()] = trimmed.slice(at + 1);
  }
  return out;
}

export function writeVariables(env: Record<string, string>): string {
  return Object.entries(env)
    .map(([name, value]) => `${name}=${value}`)
    .join("\n");
}
