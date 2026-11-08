import { vi } from "vitest";

export type Recorded = {
  method: string;
  path: string;
  ifMatch: string | null;
  body?: unknown;
};

type Route = (
  method: string,
  path: string,
  body: unknown,
) => Response | Promise<Response> | undefined;

export const json = (
  body: unknown,
  status = 200,
  headers: Record<string, string> = {},
) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });

/**
 * A collection of named resources behind `base/{name}`, judging If-Match the
 * way the server does: each one has its own revision, every write moves it, a
 * stale header gets a 412 and changes nothing.
 */
export function revisedCollection<T extends { name: string }>(
  base: string,
  start: T[],
) {
  const state = { items: start, generations: new Map<string, number>() };
  let held: Promise<void> | undefined;
  const etag = (name: string) =>
    `"v1-${name}-${state.generations.get(name) ?? 1}"`;
  const bump = (name: string) =>
    state.generations.set(name, (state.generations.get(name) ?? 1) + 1);

  const route: Route = (method, path) => {
    if (!path.startsWith(`${base}/`)) return undefined;
    const name = decodeURIComponent(
      path.slice(base.length + 1).split("/")[0] ?? "",
    );
    if (method !== "GET") return undefined;
    const answer = () => {
      const found = state.items.find((item) => item.name === name);
      return found ? json(found, 200, { ETag: etag(name) }) : json({}, 404);
    };
    return held ? held.then(answer) : answer();
  };

  /** Holds every read until released, answering with what is stored then. */
  const hold = () => {
    let release = () => {};
    held = new Promise<void>((done) => (release = done));
    return () => {
      held = undefined;
      release();
    };
  };

  const write = (
    method: string,
    name: string,
    ifMatch: string | null,
    body: unknown,
  ) => {
    const found = state.items.find((item) => item.name === name);
    if (ifMatch && (!found || ifMatch !== etag(name))) {
      return json(
        { type: "fuseone:precondition-failed", title: "stale", status: 412 },
        412,
        found ? { ETag: etag(name) } : {},
      );
    }
    bump(name);
    state.items = state.items.filter((item) => item.name !== name);
    if (method === "PUT")
      state.items.push({ ...(found ?? {}), ...(body as T), name });
    return new Response(null, {
      status: 204,
      headers: method === "PUT" ? { ETag: etag(name) } : {},
    });
  };

  /** Somebody else writes, moving the revision under the screen. */
  const elsewhere = (name: string, over: Partial<T>) => {
    bump(name);
    state.items = state.items.map((item) =>
      item.name === name ? { ...item, ...over } : item,
    );
  };

  return { state, etag, route, write, elsewhere, hold, base };
}

/**
 * The network, for a test: collections answer their own paths, `others`
 * answers the rest, and every request is recorded with its If-Match.
 */
export function stubNetwork(
  collections: ReturnType<typeof revisedCollection<{ name: string }>>[],
  others: Route,
) {
  const requests: Recorded[] = [];
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const request =
      typeof input === "string"
        ? new Request(new URL(input, "http://localhost"))
        : input;
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const ifMatch = request.headers.get("If-Match");
    const text = request.method === "GET" ? "" : await request.text();
    const body = text ? (JSON.parse(text) as unknown) : undefined;
    requests.push({ method: request.method, path, ifMatch, body });
    if (path === "/me") return new Response(null, { status: 404 });
    for (const c of collections) {
      const writeOf = request.method === "PUT" || request.method === "DELETE";
      const name = path.startsWith(`${c.base}/`)
        ? decodeURIComponent(path.slice(c.base.length + 1))
        : undefined;
      if (writeOf && name !== undefined && !name.includes("/")) {
        return c.write(request.method, name, ifMatch, body);
      }
      const answered = await c.route(request.method, path, body);
      if (answered) return answered;
    }
    return (await others(request.method, path, body)) ?? json({}, 404);
  });
  return requests;
}
