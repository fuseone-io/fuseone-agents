import rehypeMermaid from "rehype-mermaid";
import { createHash } from "node:crypto";

const lightConfig = {
  theme: "neutral",
  fontFamily: "Arial, sans-serif",
};

const darkConfig = {
  theme: "dark",
  fontFamily: "Arial, sans-serif",
};

// Render both themes at build time, then let Starlight's explicit theme switch
// choose between static SVGs. rehype-mermaid's native <picture> follows only
// the operating-system preference, which can disagree with that switch.
export default function staticMermaid() {
  const render = rehypeMermaid({
    strategy: "img-svg",
    colorScheme: "light",
    mermaidConfig: lightConfig,
    dark: darkConfig,
    prefix: "fuseone-mermaid",
  });

  return async (tree, file) => {
    const sourceDigests = mermaidSourceDigests(tree);
    await render(tree, file);
    const rewritten = { count: 0 };
    rewriteThemePictures(tree, sourceDigests, rewritten);
    if (rewritten.count !== sourceDigests.length) {
      throw new Error(
        `rendered ${rewritten.count} of ${sourceDigests.length} Mermaid diagrams in ${file.path}`
      );
    }
  };
}

function rewriteThemePictures(node, sourceDigests, rewritten) {
  if (!Array.isArray(node.children)) return;

  node.children = node.children.map((child) => {
    if (isMermaidPicture(child)) {
      const digest = sourceDigests[rewritten.count];
      rewritten.count++;
      return themedDiagram(child, digest);
    }
    rewriteThemePictures(child, sourceDigests, rewritten);
    return child;
  });
}

function mermaidSourceDigests(tree) {
  const sources = [];
  collectMermaidSources(tree, sources);
  return sources.map((source) => digest(source.trim()));
}

function collectMermaidSources(node, sources) {
  if (node?.type === "element" && node.tagName === "code") {
    const classes = Array.isArray(node.properties?.className)
      ? node.properties.className
      : [node.properties?.className];
    if (classes.includes("language-mermaid")) sources.push(textOf(node));
  }
  if (!Array.isArray(node.children)) return;
  for (const child of node.children) collectMermaidSources(child, sources);
}

function textOf(node) {
  if (node.type === "text") return node.value;
  if (!Array.isArray(node.children)) return "";
  return node.children.map(textOf).join("");
}

function digest(source) {
  return createHash("sha256").update(source).digest("hex");
}

function isMermaidPicture(node) {
  if (node?.type !== "element" || node.tagName !== "picture") return false;
  const source = node.children.find((child) => child.tagName === "source");
  const image = node.children.find((child) => child.tagName === "img");
  return (
    source?.properties?.media === "(prefers-color-scheme: dark)" &&
    typeof source.properties.srcset === "string" &&
    typeof image?.properties?.src === "string" &&
    String(image.properties.id).startsWith("fuseone-mermaid-")
  );
}

function themedDiagram(picture, sourceDigest) {
  const source = picture.children.find((child) => child.tagName === "source");
  const light = picture.children.find((child) => child.tagName === "img");
  const common = {
    alt: light.properties.alt,
    decoding: "async",
    loading: "lazy",
    title: light.properties.title,
  };

  return {
    type: "element",
    tagName: "figure",
    properties: {
      className: ["mermaid-diagram"],
      dataMermaidDigest: sourceDigest,
    },
    children: [
      {
        type: "element",
        tagName: "img",
        properties: {
          ...common,
          className: ["mermaid-diagram__image", "mermaid-diagram__image--light"],
          height: light.properties.height,
          id: light.properties.id,
          src: light.properties.src,
          width: light.properties.width,
        },
        children: [],
      },
      {
        type: "element",
        tagName: "img",
        properties: {
          ...common,
          className: ["mermaid-diagram__image", "mermaid-diagram__image--dark"],
          height: source.properties.height,
          id: source.properties.id,
          src: source.properties.srcset,
          width: source.properties.width,
        },
        children: [],
      },
    ],
  };
}
