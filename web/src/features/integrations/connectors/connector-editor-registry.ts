import {
  Database,
  Globe,
  KeyRound,
  ScrollText,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import type { ComponentType } from "react";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorInstanceForm } from "@/features/integrations/connectors/connector-instance-form";
import { CloudflareInstanceForm } from "@/features/integrations/connectors/cloudflare-instance-form";
import { ElasticsearchInstanceForm } from "@/features/integrations/connectors/elasticsearch-instance-form";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import { GraviteeInstanceEditor } from "@/features/integrations/connectors/gravitee-instance-editor";
import { SQLInstanceEditor } from "@/features/integrations/connectors/sql-instance-editor";

export type ConnectorEditorProps = {
  instance: ConnectorInstance | null;
  instances: ConnectorInstance[];
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
};

type EditorDefinition = {
  connector: string;
  label: string;
  icon: LucideIcon;
  Editor: ComponentType<ConnectorEditorProps>;
};

const editors = [
  {
    connector: "vault",
    label: "connectors.newVault",
    icon: ShieldCheck,
    Editor: ConnectorInstanceForm,
  },
  {
    connector: "sql",
    label: "connectors.newSQL",
    icon: Database,
    Editor: SQLInstanceEditor,
  },
  {
    connector: "gravitee",
    label: "connectors.newGravitee",
    icon: KeyRound,
    Editor: GraviteeInstanceEditor,
  },
  {
    connector: "cloudflare",
    label: "connectors.newCloudflare",
    icon: Globe,
    Editor: CloudflareInstanceForm,
  },
  {
    connector: "elasticsearch",
    label: "connectors.newElasticsearch",
    icon: ScrollText,
    Editor: ElasticsearchInstanceForm,
  },
] as const satisfies readonly EditorDefinition[];

export type ConfigurableConnector = (typeof editors)[number]["connector"];

export const connectorEditorOptions = editors;

export function connectorEditorFor(connector: string) {
  return editors.find((candidate) => candidate.connector === connector);
}
