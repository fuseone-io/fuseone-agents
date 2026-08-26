import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://fuseone-io.github.io",
  base: "/fuseone-agents/docs",
  integrations: [
    starlight({
      title: "FuseOne Agents",
      description:
        "Governed runtime and control plane for AI agents inside business operations.",
      customCss: ["./src/styles/fuseone.css"],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/fuseone-io/fuseone-agents",
        },
      ],
      editLink: {
        baseUrl: "https://github.com/fuseone-io/fuseone-agents/edit/main/docs/site/",
      },
      sidebar: [
        {
          label: "Start",
          items: [
            { label: "What is FuseOne?", link: "/" },
            { label: "Install", link: "/start/install/" },
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
          label: "Reference",
          items: [
            { label: "Manual and design notes", link: "/reference/manual-and-design-notes/" },
          ],
        },
      ],
    }),
  ],
});
