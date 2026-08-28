import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";

const siteRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const page = path.join(
  siteRoot,
  "dist",
  "design",
  "nt-010-the-shape-of-the-platform",
  "index.html"
);
const html = await readFile(page, "utf8");
const source = await readFile(
  path.join(
    siteRoot,
    "src",
    "content",
    "docs",
    "design",
    "nt-010-the-shape-of-the-platform.md"
  ),
  "utf8"
);
const sourceDigests = [...source.matchAll(/```mermaid\n([\s\S]*?)\n```/g)].map((match) =>
  createHash("sha256").update(match[1].trim()).digest("hex")
);

const figures = html.match(/<figure class="mermaid-diagram"[^>]*>[\s\S]*?<\/figure>/g) ?? [];
if (sourceDigests.length !== 3 || figures.length !== sourceDigests.length) {
  throw new Error(
    `NT-010 has ${sourceDigests.length} Mermaid sources and ${figures.length} rendered diagrams`
  );
}

if (html.includes('class="language-mermaid"') || html.includes("sequenceDiagram\n")) {
  throw new Error("NT-010 still contains an unrendered Mermaid code block");
}

for (const [index, figure] of figures.entries()) {
  if (!figure.includes(`data-mermaid-digest="${sourceDigests[index]}"`)) {
    throw new Error(`diagram ${index + 1} was not rendered from the current Mermaid source`);
  }
  for (const theme of ["light", "dark"]) {
    const image = new RegExp(
      `<img[^>]+class="mermaid-diagram__image mermaid-diagram__image--${theme}"[^>]*>`
    ).exec(figure)?.[0];
    if (!image) throw new Error(`diagram ${index + 1} has no ${theme} SVG`);
    if (!/alt="[^"]+"/.test(image)) {
      throw new Error(`diagram ${index + 1} has no accessible description`);
    }
    if (!/src="data:image\/svg\+xml,/.test(image)) {
      throw new Error(`diagram ${index + 1} ${theme} image is not a static SVG`);
    }
  }
}

if (/<script[^>]+(?:src|type)="[^"]*mermaid/i.test(html)) {
  throw new Error("the documentation ships Mermaid runtime JavaScript");
}

console.log("NT-010: 3 static, theme-aware Mermaid diagrams verified");
