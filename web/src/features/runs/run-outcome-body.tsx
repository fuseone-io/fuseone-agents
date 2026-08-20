import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/**
 * A run's closing answer, rendered as the document it is.
 *
 * The model writes these as reports — headings, tables, fenced commands — and
 * showing them as one block of preformatted text made a diagnosis harder to
 * read than the logs it came from.
 *
 * Not the manual's renderer, and the difference is not styling. The manual is
 * ours and reviewed in a pull request; this restates whatever the agent read on
 * the way, which came from outside and is why the run carries `untrusted`. So
 * the two policies below are the whole reason this component exists:
 *
 * A link is shown, never offered. Turning a destination a third party chose
 * into something an auditor can click would make the trail a place to be
 * phished from.
 *
 * An image is not fetched. `![](url)` would have the console make an outbound
 * request chosen by untrusted content, which is a read receipt for whoever
 * hosts it and a request nobody in the installation authorised.
 *
 * No raw-HTML plugin, as everywhere else: markup is never parsed as markup, so
 * there is nothing to sanitise afterwards.
 */
export function RunOutcomeBody({ body }: { body: string }) {
  return (
    <div className="min-w-0 max-w-full text-sm [&_h1]:mt-4 [&_h1]:text-base [&_h1]:font-medium [&_h2]:mt-4 [&_h2]:font-medium [&_h3]:mt-3 [&_h3]:font-medium [&_p]:my-2 [&_p]:break-words [&_li]:my-0.5 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_strong]:font-medium [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-px [&_code]:font-mono [&_code]:text-xs">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{ a: Plain, img: Nothing, pre: Block, table: Grid }}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

/** The text and where it pointed, both readable, neither clickable. */
function Plain({ href, children }: { href?: string; children?: ReactNode }) {
  return (
    <span className="break-words">
      {children}
      {href ? <span className="text-muted-foreground"> ({href})</span> : null}
    </span>
  );
}

function Nothing() {
  return null;
}

// Commands and log excerpts arrive fenced and are wider than the column.
function Block({ children }: { children?: ReactNode }) {
  return (
    <pre className="my-2 max-w-full overflow-x-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs">
      {children}
    </pre>
  );
}

function Grid({ children }: { children?: ReactNode }) {
  return (
    <div className="my-3 max-w-full overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-xs [&_td]:border-t [&_td]:border-border [&_td]:p-2 [&_th]:p-2 [&_th]:text-left [&_th]:font-medium">
        {children}
      </table>
    </div>
  );
}
