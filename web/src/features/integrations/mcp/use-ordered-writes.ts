import { useCallback, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { integrationKeys, type MCPServer } from "@/features/integrations/api";
import { staleRefusal, type Revised } from "@/lib/api/revision";

/**
 * Writes to one tool server, one after the other, each answering the revision
 * its page knows.
 *
 * The server's page writes it from two places, the surface and the
 * credential. What it knows is the revision it opened on, and after each of
 * its own writes the one that write made. A write waiting in line runs once
 * the server has been read again, and only if that read names the revision the
 * page knows: anything else is somebody else's write, which a draft made before
 * it must not be sent over, or a read that failed. Either is refused, and the
 * record sent is the one that read returned.
 */
export function useOrderedWrites(
  name: string | undefined,
  /** The revision the page opened on, or was last reloaded to. */
  opened: string | undefined,
) {
  const queryClient = useQueryClient();
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const made = useRef<string | undefined>(undefined);

  const inOrder = useCallback(
    (
      send: (current: Revised<MCPServer>) => Promise<string | undefined>,
    ): Promise<void> => {
      const run = queue.current.then(async () => {
        const current = queryClient.getQueryData<Revised<MCPServer>>(
          integrationKeys.mcpServer(name ?? ""),
        );
        if (!current) throw new Error("the server has not been read");
        // A read that failed leaves the revision before, which is never the
        // one the write before this made, so it is refused here too.
        if (current.etag !== (made.current ?? opened)) throw staleRefusal();
        // A write that named no revision leaves nothing a next one can answer.
        made.current = (await send(current)) ?? "";
      });
      queue.current = run.catch(() => undefined);
      return run;
    },
    [queryClient, name, opened],
  );
  // After a reload, what the page knows is what it was reloaded to.
  const rebase = useCallback(() => {
    made.current = undefined;
  }, []);

  return { inOrder, rebase };
}

export type InOrder = ReturnType<typeof useOrderedWrites>["inOrder"];
