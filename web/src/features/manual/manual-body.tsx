import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Link } from "react-router-dom";
import type { ManualPage } from "@/features/manual/api";

/**
 * One page of the manual, rendered.
 *
 * No raw-HTML plugin, and that is the safety story rather than an omission:
 * markup in the text is never parsed as markup, so there is nothing to
 * sanitise afterwards. Sanitising is what you do once you have decided to
 * parse HTML and then need to take most of it back.
 *
 * GFM because the manual compares things in tables, and a comparison written
 * as a list is a comparison nobody reads.
 */
export function ManualBody({
  body,
  headings = [],
}: {
  body: string;
  headings?: ManualPage["headings"];
}) {
  let headingIndex = 0;
  const Heading = ({ children }: { children?: React.ReactNode }) => {
    const heading = headings[headingIndex++];
    return (
      <h2 id={heading?.id} className="scroll-mt-20">
        {children}
      </h2>
    );
  };
  const Subheading = ({ children }: { children?: React.ReactNode }) => {
    const heading = headings[headingIndex++];
    return (
      <h3 id={heading?.id} className="scroll-mt-20">
        {children}
      </h3>
    );
  };

  return (
    <div className="min-w-0 max-w-[72ch] [&_h2]:mt-8 [&_h2]:text-lg [&_h2]:font-medium [&_h3]:mt-6 [&_h3]:font-medium [&_p]:my-3 [&_p]:break-words [&_li]:my-1 [&_ul]:list-disc [&_ul]:pl-5 [&_strong]:font-medium">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{ a: Anchor, table: Grid, h2: Heading, h3: Subheading }}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

/**
 * A link, resolved for wherever it is being read.
 *
 * The same text is read on GitHub and in the console. There a cross-reference
 * is a path to a file; here it has to be a route, because the console serves
 * the manual from an endpoint that knows nothing about the repository.
 */
function Anchor({ href, children }: { href?: string; children?: React.ReactNode }) {
  const target = href ?? "";
  if (/^https?:|^mailto:/.test(target)) {
    return (
      <a
        href={target}
        target="_blank"
        rel="noopener noreferrer"
        className="text-primary underline underline-offset-2"
      >
        {children}
      </a>
    );
  }
  return (
    <Link
      to={`/manual/${target.replace(/\.md$/, "")}`}
      className="text-primary underline underline-offset-2"
    >
      {children}
    </Link>
  );
}

// A table wide enough to compare four rungs is wider than a phone. It scrolls
// inside its own box rather than taking the page with it — the same rule the
// approval card had to learn.
function Grid({ children }: { children?: React.ReactNode }) {
  return (
    <div className="my-4 max-w-full overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-sm [&_td]:border-t [&_td]:border-border [&_td]:p-2.5 [&_th]:p-2.5 [&_th]:text-left [&_th]:font-medium">
        {children}
      </table>
    </div>
  );
}
