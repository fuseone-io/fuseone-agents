/**
 * The tools the platform serves itself, offered whatever the step
 * (`domain.PlatformTools`). Explicit, as the server's set is: a prefix would
 * exempt a typo of one, or a vendor squatting the sigil, and the console would
 * stay quiet about a tool the server refuses. A Go test holds this list to the
 * server's, both ways.
 */
export const PLATFORM_TOOLS: ReadonlySet<string> = new Set([
  "$fuseone.context.read",
  "$fuseone.ticket.answer",
  "$fuseone.memory.find",
  "$fuseone.memory.suggest",
  "$fuseone.work_item.claim",
  "$fuseone.work_item.link",
]);
