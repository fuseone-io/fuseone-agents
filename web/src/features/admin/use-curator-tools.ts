import { useClassifiableTools, useTools } from "@/features/admin/api";
import { useMe } from "@/features/session/api";

/**
 * The tools a curator's queue is drawn from (NT-019).
 *
 * What the caller governs, asked once: their own scopes' tools and their
 * areas', every tool for the platform's curator. With no identity configured
 * at all (`null`), every door is open and the whole catalogue is asked for.
 * Nothing is asked until the session answers.
 */
export function useCuratorTools() {
  const me = useMe();
  const known = me.data !== undefined;
  const open = me.data === null;
  const governed = useClassifiableTools(known && !open);
  const everything = useTools(known && open);
  const answer = open ? everything : governed;
  return {
    me: me.data,
    data: answer.data,
    isLoading: me.isLoading || answer.isLoading,
    error: me.error ?? answer.error,
    refetch: () => void answer.refetch(),
  };
}
