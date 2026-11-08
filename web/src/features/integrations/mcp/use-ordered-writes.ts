import { useCallback, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { integrationKeys, type MCPServer } from "@/features/integrations/api";
import type { Revised } from "@/lib/api/revision";

/**
 * Writes to one tool server, one after the other, each with the record and
 * the revision stored when it runs.
 *
 * The server's page writes it from two places, the surface and the
 * credential. Each write moves its revision and is done only once the server
 * has been read again, so a second write started meanwhile waits for that read
 * instead of answering the revision before, which would have it refused over
 * a change of its own. Record and revision come from the same read.
 */
export function useOrderedWrites(name: string | undefined) {
  const queryClient = useQueryClient();
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  return useCallback(
    <T>(send: (current: Revised<MCPServer>) => Promise<T>): Promise<T> => {
      const run = queue.current.then(() => {
        const current = queryClient.getQueryData<Revised<MCPServer>>(
          integrationKeys.mcpServer(name ?? ""),
        );
        if (!current) throw new Error("the server has not been read");
        return send(current);
      });
      queue.current = run.catch(() => undefined);
      return run;
    },
    [queryClient, name],
  );
}

export type InOrder = ReturnType<typeof useOrderedWrites>;
