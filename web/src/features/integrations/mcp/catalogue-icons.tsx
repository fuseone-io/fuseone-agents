import {
  Activity,
  BookOpen,
  Brain,
  Bug,
  Calendar,
  ChartNoAxesCombined,
  Clock,
  Cloud,
  CloudCog,
  CreditCard,
  Database,
  FileSpreadsheet,
  FileText,
  Folder,
  GitBranch,
  GitPullRequest,
  Globe,
  HardDrive,
  ListChecks,
  Mail,
  MessageSquare,
  Presentation,
  Search,
  Server,
  SquareKanban,
  Terminal,
  Users,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";

import type { Listing } from "@/features/integrations/mcp/catalogue";

type IconSource = Pick<Listing, "name" | "category">;

const SERVER_ICONS: Record<string, LucideIcon> = {
  atlassian: ListChecks,
  aws: Server,
  "aws-cloudwatch": CloudCog,
  "aws-knowledge": BookOpen,
  "aws-pricing": CreditCard,
  "aws-prometheus": Activity,
  custom: Server,
  datadog: Activity,
  fetch: Search,
  filesystem: Folder,
  gcloud: Cloud,
  "gcp-databases": Database,
  git: GitBranch,
  github: GitPullRequest,
  "google-calendar": Calendar,
  "google-chat": MessageSquare,
  "google-cloud-observability": CloudCog,
  "google-docs": FileText,
  "google-drive": HardDrive,
  "google-gmail": Mail,
  "google-people": Users,
  "google-sheets": FileSpreadsheet,
  "google-slides": Presentation,
  grafana: ChartNoAxesCombined,
  linear: SquareKanban,
  memory: Brain,
  notion: BookOpen,
  postgres: Database,
  sentry: Bug,
  slack: MessageSquare,
  stripe: CreditCard,
  time: Clock,
};

const CATEGORY_ICONS: Record<string, LucideIcon> = {
  code: Terminal,
  communication: MessageSquare,
  data: Database,
  finance: CreditCard,
  knowledge: BookOpen,
  operations: Activity,
  web: Globe,
};

export function CatalogueIcon({
  entry,
  className,
}: {
  entry: IconSource;
  className?: string;
}) {
  const Icon = SERVER_ICONS[entry.name] ?? CATEGORY_ICONS[entry.category] ?? Server;

  return (
    <span
      className={cn(
        "flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground",
        className,
      )}
      data-mcp-icon={entry.name}
      aria-hidden
    >
      <Icon className="size-5" />
    </span>
  );
}
