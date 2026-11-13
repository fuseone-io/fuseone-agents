import type { MCPServer } from "@/features/integrations/api";

/**
 * Whether the caller may configure a catalogue entry (NT-019).
 *
 * A recipe nobody connected needs only the right to create somewhere. A
 * connected server needs more: the caller may change it (`editable`), and it
 * is one the console manages at all (`managed`), since one passed to the
 * process by flag or environment has no row to change. Absent means an older
 * backend, which answered for the platform alone.
 */
export function configurable(canCreate: boolean, server: MCPServer | null | undefined): boolean {
  if (!canCreate) return false;
  return !server || (server.editable !== false && server.managed !== false);
}
