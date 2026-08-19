import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { KeyRound, MessageSquare, PackageOpen, Plug, Server } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type IntegrationSection =
  | "connected"
  | "available"
  | "credentials"
  | "providers"
  | "channels";

export type IntegrationCounts = Partial<Record<IntegrationSection, number>>;

const sections: {
  id: IntegrationSection;
  label: string;
  path: string;
  icon: typeof Server;
}[] = [
  {
    id: "connected",
    label: "integrations.connected",
    path: "/integrations",
    icon: Server,
  },
  {
    id: "available",
    label: "integrations.available",
    path: "/integrations/mcp",
    icon: PackageOpen,
  },
  {
    id: "credentials",
    label: "integrations.credentials",
    path: "/integrations/credentials",
    icon: KeyRound,
  },
  {
    id: "providers",
    label: "integrations.providers",
    path: "/integrations/providers",
    icon: Plug,
  },
  {
    id: "channels",
    label: "channels.channels",
    path: "/integrations/channels",
    icon: MessageSquare,
  },
];

/**
 * The four jobs under integrations, with one side rail.
 *
 * Horizontal tabs made providers, tool servers and channels look like peer
 * settings inside one page, while the MCP catalogue lived somewhere else.
 * This rail keeps the shape of the product visible: what is already connected,
 * what is available to connect, who can plan, and where runs speak.
 */
export function IntegrationsShell({
  active,
  counts,
  children,
}: {
  active: IntegrationSection;
  counts: IntegrationCounts;
  children: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <div className="grid gap-6 lg:grid-cols-[13rem_minmax(0,1fr)]">
      <nav
        aria-label={t("integrations.sections")}
        className="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0"
      >
        {sections.map((section) => {
          const Icon = section.icon;
          const activeHere = section.id === active;
          const count = counts[section.id];
          return (
            <Button
              key={section.id}
              asChild
              variant="ghost"
              size="sm"
              className={cn(
                "h-9 min-w-max justify-start px-2 lg:w-full",
                activeHere && "bg-muted font-medium",
              )}
            >
              <Link
                to={section.path}
                aria-current={activeHere ? "page" : undefined}
              >
                <Icon className="size-4" aria-hidden />
                <span className="truncate">{t(section.label)}</span>
                {typeof count === "number" && (
                  <span className="ml-auto rounded-md bg-muted px-1.5 font-mono text-2xs tabular-nums text-muted-foreground">
                    {count}
                  </span>
                )}
              </Link>
            </Button>
          );
        })}
      </nav>

      <div className="min-w-0">{children}</div>
    </div>
  );
}
