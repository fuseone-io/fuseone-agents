// The generator's output agrees with the lists it was given: every published
// note is there, no unpublished note is, and no page links to an unpublished
// note as if it were on the site — such a link would be a dead page.
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { designDocs, unpublishedDesignDocs } from "./design-notes.mjs";

const siteRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const designOut = path.join(siteRoot, "src", "content", "docs", "design");
const slug = (file) => path.basename(file, ".md").toLowerCase();

const generated = new Set((await readdir(designOut)).map((file) => path.basename(file, ".md")));
const problems = [];

for (const [file] of designDocs) {
  const published = !unpublishedDesignDocs.has(file);
  if (published && !generated.has(slug(file))) problems.push(`${file} is listed but was not generated`);
  if (!published && generated.has(slug(file))) problems.push(`${file} is unpublished but was generated`);
}

for (const page of generated) {
  const body = await readFile(path.join(designOut, `${page}.md`), "utf8");
  for (const file of unpublishedDesignDocs) {
    const target = `(${file.replace(/\./g, "\\.")}|${slug(file)}/)`;
    if (new RegExp(`\\]\\((?!https?:)[^)]*${target}`).test(body)) {
      problems.push(`${page} links to unpublished ${file} as a page on this site`);
    }
  }
}

if (problems.length > 0) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`design notes: ${generated.size} generated, ${unpublishedDesignDocs.size} deliberately unpublished`);
