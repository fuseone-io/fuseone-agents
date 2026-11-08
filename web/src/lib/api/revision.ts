import { ApiError, unwrap } from "@/lib/api/client";

/**
 * A resource as it was read, and the revision a write over it answers.
 *
 * The two travel together because they are one fact: a form seeded from one
 * read and saved under the revision of another would claim to have seen a
 * state it never showed anybody.
 */
export type Revised<T> = { body: T; etag: string };

/** The revision a form read, and how to read the stored version again. */
export type FormRevision = {
  etag: string;
  reload: () => void;
  reloading: boolean;
};

export function revised<T>(result: {
  data?: T;
  error?: unknown;
  response: Response;
}): Revised<T> {
  const body = unwrap(result);
  const etag = result.response.headers.get("ETag");
  // Fails closed. Read as no revision, the next write would send no If-Match
  // and quietly go back to last-write-wins; a resource that exists has one.
  if (!etag) {
    throw new ApiError(result.response.status, {
      type: "fuseone:revision-missing",
      title: "The answer carried no revision",
      status: result.response.status,
    });
  }
  return { body, etag };
}

/**
 * The precondition for a write over what was read. A first write answers none:
 * the server refuses any If-Match on something that does not exist.
 */
export function ifMatch(etag: string | undefined): { "If-Match"?: string } {
  return etag ? { "If-Match": etag } : {};
}

/**
 * How a read that seeds an editor is kept: read again whenever an editor
 * opens, and at no other moment but a reload somebody asked for. A read on
 * focus or on reconnect would bring a newer revision under a form being
 * filled in and replace it, edits and all, without saying so.
 */
export const revisionReads = {
  refetchOnMount: "always",
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;

/** Somebody else wrote since this was read, and nothing was saved. */
export function isStale(error: unknown): boolean {
  return error instanceof ApiError && error.status === 412;
}
