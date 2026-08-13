import {
  useInfiniteQuery,
  type QueryKey,
  type UndefinedInitialDataInfiniteOptions,
} from "@tanstack/react-query";

/**
 * A list the server hands back one page at a time.
 *
 * The pages are cursor-based, not numbered, and the UI follows suit: there is
 * no "page 4 of 27" because the server never counts the set to answer a page.
 * That is the point — a count over the whole history is what makes a list get
 * slower as an installation gets older.
 *
 * Numbering by offset would have been less code and would also be wrong here.
 * These lists are newest first and rows arrive while somebody reads, so page
 * two at offset fifty is not the fifty-first row — it is whatever the first
 * fifty have been pushed down to, which means rows repeat and rows vanish.
 */
export interface Page<T> {
  items: T[];
  nextCursor?: string | null;
}

export function usePagedQuery<T>(
  key: QueryKey,
  fetchPage: (cursor?: string) => Promise<Page<T>>,
  options: Partial<UndefinedInitialDataInfiniteOptions<Page<T>, Error>> = {},
) {
  const query = useInfiniteQuery({
    queryKey: key,
    queryFn: ({ pageParam }) => fetchPage(pageParam as string | undefined),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last: Page<T>) => last.nextCursor ?? undefined,
    ...options,
  });

  return {
    ...query,
    /** Every row fetched so far, in order. Components render this and never
     *  reach into the page structure. */
    items: (query.data?.pages ?? []).flatMap((page) => page.items),
  };
}
