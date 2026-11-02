/**
 * Where a work item's trail lives in the console. The reference is encoded
 * whole: a raw `#` would end the URL's path at the issue number.
 */
export function workItemPath(ref: string): string {
  return `/work-items?ref=${encodeURIComponent(ref)}`;
}
