import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const siteRoot = path.resolve(scriptDir, "..");
const docsRoot = path.resolve(siteRoot, "..");
const contentRoot = path.join(siteRoot, "src", "content", "docs");
const designOut = path.join(contentRoot, "design");
const manualOut = path.join(contentRoot, "manual");
const helmOut = path.join(contentRoot, "reference", "helm-chart.md");

const designDocs = [
  ["PRD-001-fuseone-agents.md", "Product requirements and product vocabulary."],
  ["DP-001-data-protection.md", "Stored data, retention, erasure and boundaries."],
  ["OP-001-running-an-installation.md", "Installation and operation guide."],
  ["NT-001-integration-boundary-and-execution-model.md", "Where MCP ends and integration begins."],
  ["NT-002-remaining-work.md", "Remaining product work and ordering."],
  ["NT-003-conversational-authoring.md", "Authoring an agent by conversation."],
  ["NT-004-ledger-volume-and-paging.md", "Ledger volume, partitions and paging."],
  ["NT-005-interaction-channels.md", "Channels and their product boundaries."],
  ["NT-006-evaluating-agents.md", "Evaluating agents and simulation strategy."],
  ["NT-007-drawing-a-process.md", "Process canvas and authored stages."],
  ["NT-008-a-catalogue-by-shape.md", "Tool catalogue chosen by shape."],
  ["NT-009-governed-connectors.md", "Governed connector shapes and runtime boundary."],
  ["NT-010-the-shape-of-the-platform.md", "Topology, the run loop, layering and where data is written."],
  ["NT-011-durable-agent-execution-and-workflow-engines.md", "Durable agent execution and the workflow-engine boundary."],
];

const manualLocales = [
  {
    source: "en-US",
    route: "en-us",
    title: "English manual",
    description: "The in-product manual rendered for public documentation.",
  },
  {
    source: "pt-BR",
    route: "pt-br",
    title: "Manual em portugues",
    description: "O manual do produto renderizado para documentacao publica.",
  },
];

const designSlugByFile = new Map(designDocs.map(([file]) => [file, slugFromFile(file)]));

await rm(designOut, { recursive: true, force: true });
await rm(manualOut, { recursive: true, force: true });
await rm(helmOut, { force: true });
await mkdir(designOut, { recursive: true });
await mkdir(manualOut, { recursive: true });

await writeManualIndex();
await writeDesignDocs();
await writeManualDocs();
await writeHelmDoc();

async function writeDesignDocs() {
  let order = 1;
  for (const [file, description] of designDocs) {
    const raw = await readFile(path.join(docsRoot, file), "utf8");
    const { title, body } = stripFirstHeading(raw);
    const slug = designSlugByFile.get(file);
    await writeDoc(path.join(designOut, `${slug}.md`), {
      title,
      description,
      order,
      body: transformDesignLinks(body, "../"),
    });
    order++;
  }
}

async function writeManualDocs() {
  for (const locale of manualLocales) {
    const source = path.join(docsRoot, "manual", locale.source);
    const target = path.join(manualOut, locale.route);
    await mkdir(target, { recursive: true });

    const pages = [];
    for (const file of await sortedMarkdownFiles(source)) {
      const raw = await readFile(path.join(source, file), "utf8");
      const { meta, body } = parseManualPage(raw, file);
      const slug = slugFromFile(file);
      pages.push({ slug, title: meta.title, summary: meta.summary, order: Number(meta.order) });
      await writeDoc(path.join(target, `${slug}.md`), {
        title: meta.title,
        description: meta.summary,
        order: Number(meta.order),
        body: transformManualLinks(body),
      });
    }

    pages.sort((a, b) => a.order - b.order || a.title.localeCompare(b.title));
    await writeDoc(path.join(target, "index.md"), {
      title: locale.title,
      description: locale.description,
      order: 0,
      body: [
        "These pages are generated from the same Markdown files embedded in the product console.",
        "",
        ...pages.map((page) => `- [${page.title}](./${page.slug}/) - ${page.summary}`),
      ].join("\n"),
    });
  }
}

async function writeManualIndex() {
  await writeDoc(path.join(manualOut, "index.md"), {
    title: "Product manual",
    description: "The in-product manual rendered as public documentation.",
    order: 1,
    body: [
      "FuseOne ships a manual inside the console. The same source pages are rendered here for public reading.",
      "",
      ...manualLocales.map((locale) => `- [${locale.title}](./${locale.route}/)`),
      "",
      "The source of truth remains `docs/manual`; this site is generated from it during the docs build.",
    ].join("\n"),
  });
}

async function writeHelmDoc() {
  const raw = await readFile(
    path.join(docsRoot, "..", "deploy", "helm", "fuseone-agents", "README.md"),
    "utf8"
  );
  const { title, body } = stripFirstHeading(raw);
  await writeDoc(helmOut, {
    title,
    description: "Helm chart installation and operation reference.",
    order: 2,
    body: transformDesignLinks(body, "../../design/"),
  });
}

async function writeDoc(file, { title, description, order, body }) {
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(
    file,
    `${frontMatter({ title, description, order })}\n${body.trim()}\n`,
    "utf8"
  );
}

function frontMatter({ title, description, order }) {
  return [
    "---",
    `title: ${quoted(title)}`,
    `description: ${quoted(description)}`,
    "sidebar:",
    `  label: ${quoted(title)}`,
    `  order: ${Number.isFinite(order) ? order : 999}`,
    "---",
  ].join("\n");
}

function quoted(value) {
  return JSON.stringify(String(value).replace(/\s+/g, " ").trim());
}

async function sortedMarkdownFiles(dir) {
  const files = (await readdir(dir)).filter((file) => file.endsWith(".md"));
  return files.sort((a, b) => a.localeCompare(b));
}

function parseManualPage(raw, file) {
  const match = /^---\n([\s\S]*?)\n---\n([\s\S]*)$/.exec(raw);
  if (!match) {
    throw new Error(`manual page ${file} has no front matter`);
  }
  const meta = {};
  for (const line of match[1].split("\n")) {
    const at = line.indexOf(":");
    if (at < 0) continue;
    meta[line.slice(0, at).trim()] = line.slice(at + 1).trim();
  }
  for (const key of ["title", "summary", "order"]) {
    if (!meta[key]) {
      throw new Error(`manual page ${file} is missing ${key}`);
    }
  }
  return { meta, body: match[2] };
}

function stripFirstHeading(raw) {
  const lines = raw.split("\n");
  const titleLine = lines.findIndex((line) => line.startsWith("# "));
  if (titleLine < 0) {
    throw new Error("source document has no h1");
  }
  const title = lines[titleLine].replace(/^#\s+/, "").trim();
  lines.splice(titleLine, 1);
  while (lines[0] === "") lines.shift();
  return { title, body: lines.join("\n") };
}

function slugFromFile(file) {
  return path.basename(file, ".md").toLowerCase();
}

function transformManualLinks(body) {
  return body.replace(
    /\]\((?!https?:|mailto:|#|\/)([^)#]+)\.md(#[^)]+)?\)/g,
    (_match, target, anchor = "") => `](../${target}/${anchor})`
  );
}

function transformDesignLinks(body, prefix) {
  return body.replace(/\]\(([^)\s]+\.md)(#[^)]+)?\)/g, (match, target, anchor = "") => {
    const slug = designSlugByFile.get(path.basename(target));
    if (!slug) return match;
    return `](${prefix}${slug}/${anchor})`;
  });
}
