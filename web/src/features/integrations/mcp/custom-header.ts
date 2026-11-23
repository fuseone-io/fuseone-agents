/**
 * One header a remote server with no recipe is reached with: a name and the
 * value sent under it, such as `apikey: <key>`. It is the credential, so it
 * replaces a stored bearer or OAuth grant, as the server does with any header.
 */
export type CustomHeader = { name: string; value: string };

/** Saves a header credential, and says whether it was stored. */
export type WriteHeaders = (credential: { headers: Record<string, string> }) => Promise<boolean>;

/**
 * Letters, digits and `- _ .`: every header an API names its key with, and
 * nothing HTTP would refuse to send. The server judges the full rule.
 */
const HEADER_NAME = /^[A-Za-z0-9._-]+$/;

export type CustomHeaderState = "empty" | "incomplete" | "invalid" | "complete";

export function customHeaderState(header: CustomHeader): CustomHeaderState {
  const name = header.name.trim();
  const value = header.value.trim();
  if (name === "" && value === "") return "empty";
  if (name === "" || value === "") return "incomplete";
  return HEADER_NAME.test(name) ? "complete" : "invalid";
}

/** What a complete header sends: the trimmed name, the value as typed. */
export function customHeaderCredential(header: CustomHeader): Record<string, string> {
  return { [header.name.trim()]: header.value };
}

/** The message a header that cannot be saved shows, or none. */
export function customHeaderError(header: CustomHeader): string | undefined {
  switch (customHeaderState(header)) {
    case "incomplete":
      return "mcp.customHeaderIncomplete";
    case "invalid":
      return "mcp.customHeaderNameInvalid";
    default:
      return undefined;
  }
}
