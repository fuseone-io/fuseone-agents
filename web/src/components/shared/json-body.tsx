import { Fragment, type ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * The largest payload worth colouring.
 *
 * Colouring emits a node per value, which is nothing for a tool result and a
 * frozen tab for a dump somebody pasted. Above this the payload is still shown
 * in full and still scrolls — it simply arrives as text.
 */
const MAX_COLOURED = 64 * 1024;

/**
 * A tool's arguments or result, as the trail shows them.
 *
 * Rendered from the parsed value rather than by matching patterns in the
 * string, so what is coloured is what the payload actually is. Anything that
 * does not parse is shown as it arrived: a tool returns what it returns, and a
 * console that blanked an error in prose would be losing the evidence somebody
 * opened the step to read.
 *
 * Every value is a text node. The payload came from outside the platform and
 * is untrusted by definition; React escapes text, so there is nothing here to
 * interpret and nothing to sanitise. Reaching for innerHTML to make colouring
 * cheaper would undo exactly that.
 */
export function JsonBody({ body, className }: { body: string; className?: string }) {
  const shell = cn(
    "max-h-[min(48vh,28rem)] max-w-full overflow-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs whitespace-pre-wrap break-words",
    className,
  );

  if (body.length > MAX_COLOURED) {
    return <pre className={shell}>{body}</pre>;
  }

  let value: unknown;
  try {
    value = JSON.parse(body);
  } catch {
    return <pre className={shell}>{body}</pre>;
  }
  return <pre className={shell}>{render(value, 0)}</pre>;
}

const INDENT = "  ";

/**
 * JSON's own word for nothing, held as a value rather than written as text.
 *
 * The string checks are right to stop a literal in a component, and this is
 * the exception worth naming rather than exempting: it is syntax the payload
 * contains, not copy somebody reads, and a translated `null` would make the
 * trail disagree with the bytes it claims to be showing.
 */
const NULL = "null";

function render(value: unknown, depth: number): ReactNode {
  if (value === null) return <span className="text-warning">{NULL}</span>;
  switch (typeof value) {
    case "string":
      return <span className="text-success">{JSON.stringify(value)}</span>;
    case "number":
      return <span className="text-primary tabular-nums">{String(value)}</span>;
    case "boolean":
      return <span className="text-warning">{String(value)}</span>;
  }
  if (Array.isArray(value)) return list(value, depth);
  if (typeof value === "object") return object(value as Record<string, unknown>, depth);
  return <span>{String(value)}</span>;
}

function list(items: unknown[], depth: number): ReactNode {
  if (items.length === 0) return <span>[]</span>;
  const pad = INDENT.repeat(depth + 1);
  return (
    <>
      {"[\n"}
      {items.map((item, at) => (
        <Fragment key={at}>
          {pad}
          {render(item, depth + 1)}
          {at < items.length - 1 ? ",\n" : "\n"}
        </Fragment>
      ))}
      {INDENT.repeat(depth)}
      {"]"}
    </>
  );
}

function object(entries: Record<string, unknown>, depth: number): ReactNode {
  const keys = Object.keys(entries);
  if (keys.length === 0) return <span>{"{}"}</span>;
  const pad = INDENT.repeat(depth + 1);
  return (
    <>
      {"{\n"}
      {keys.map((key, at) => (
        <Fragment key={key}>
          {pad}
          {/* Muted rather than coloured: reading a payload is mostly finding
              the key you want, and a key that competes with its value for
              attention makes both harder to scan. */}
          <span className="text-muted-foreground">{JSON.stringify(key)}</span>
          {": "}
          {render(entries[key], depth + 1)}
          {at < keys.length - 1 ? ",\n" : "\n"}
        </Fragment>
      ))}
      {INDENT.repeat(depth)}
      {"}"}
    </>
  );
}
