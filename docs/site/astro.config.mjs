import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const siteRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(siteRoot, "..", "..");

function manualSidebar(sourceLocale, route, label) {
  return {
    label,
    items: [
      { label: `${label} overview`, link: `/manual/${route}/` },
      ...manualPages(sourceLocale, route),
    ],
  };
}

function manualPages(sourceLocale, route) {
  const source = path.join(repoRoot, "docs", "manual", sourceLocale);
  return readdirSync(source)
    .filter((file) => file.endsWith(".md"))
    .map((file) => {
      const raw = readFileSync(path.join(source, file), "utf8");
      const meta = frontMatter(raw, file);
      const order = Number(meta.order);
      if (!Number.isFinite(order)) {
        throw new Error(`manual page ${file} has a non-numeric order`);
      }
      return {
        label: meta.title,
        link: `/manual/${route}/${path.basename(file, ".md").toLowerCase()}/`,
        order,
      };
    })
    .sort((a, b) => a.order - b.order || a.label.localeCompare(b.label))
    .map(({ label, link }) => ({ label, link }));
}

function frontMatter(raw, file) {
  const match = /^---\n([\s\S]*?)\n---\n/.exec(raw);
  if (!match) {
    throw new Error(`manual page ${file} has no front matter`);
  }
  const meta = {};
  for (const line of match[1].split("\n")) {
    const at = line.indexOf(":");
    if (at < 0) continue;
    meta[line.slice(0, at).trim()] = line.slice(at + 1).trim();
  }
  for (const key of ["title", "order"]) {
    if (!meta[key]) {
      throw new Error(`manual page ${file} is missing ${key}`);
    }
  }
  return meta;
}

export default defineConfig({
  site: "https://fuseone-io.github.io",
  base: "/fuseone-agents/docs",
  integrations: [
    starlight({
      title: "FuseOne Agents",
      description:
        "Governed runtime and control plane for AI agents inside business operations.",
      customCss: ["./src/styles/fuseone.css"],
      logo: {
        light: "./src/assets/logo-docs-light.svg",
        dark: "./src/assets/logo-docs-dark.svg",
        replacesTitle: true,
      },
      head: [
        {
          tag: "link",
          attrs: {
            rel: "icon",
            type: "image/svg+xml",
            href: "/fuseone-agents/docs/favicon.svg",
          },
        },
      ],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/fuseone-io/fuseone-agents",
        },
      ],
      sidebar: [
        {
          label: "Start",
          items: [
            { label: "What is FuseOne?", link: "/" },
            { label: "Install", link: "/start/install/" },
            { label: "Helm chart", link: "/reference/helm-chart/" },
          ],
        },
        {
          label: "Core concepts",
          items: [
            { label: "Gate and labels", link: "/concepts/gate/" },
            { label: "Integrations", link: "/concepts/integrations/" },
            { label: "Duplicate effects", link: "/concepts/duplicate-effects/" },
            { label: "Governed memory", link: "/concepts/memory/" },
          ],
        },
        {
          label: "Operate",
          items: [
            { label: "Runtime and FinOps", link: "/operate/runtime-finops/" },
          ],
        },
        {
          label: "Manual",
          items: [
            { label: "Manual overview", link: "/manual/" },
            manualSidebar("en-US", "en-us", "English"),
            manualSidebar("pt-BR", "pt-br", "Portuguese"),
          ],
        },
        {
          label: "Design record",
          items: [
            { label: "Manual and design notes", link: "/reference/manual-and-design-notes/" },
            { label: "Rendered source docs", items: [{ autogenerate: { directory: "design" } }] },
          ],
        },
      ],
    }),
  ],
});
