# FuseOne Agents docs site

This is the public documentation site published to GitHub Pages.

```sh
npm ci
npm run generate
npm run build
npm run preview
```

The source of truth for the in-product manual remains `docs/manual`. This site
is the public navigation layer: it introduces the product, links to the design
record, and gives operators a readable path through the core concepts.

`npm run generate` copies the repository Markdown sources into Starlight pages.
The generated pages are ignored by Git so the public site cannot drift from the
manual and design notes reviewers read in pull requests.
