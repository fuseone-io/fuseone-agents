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
  return {
    body: unwrap(result),
    etag: result.response.headers.get("ETag") ?? "",
  };
}

/**
 * The precondition for a write over what was read. A first write answers none:
 * the server refuses any If-Match on something that does not exist.
 */
export function ifMatch(etag: string | undefined): { "If-Match"?: string } {
  return etag ? { "If-Match": etag } : {};
}

/** Somebody else wrote since this was read, and nothing was saved. */
export function isStale(error: unknown): boolean {
  return error instanceof ApiError && error.status === 412;
}
