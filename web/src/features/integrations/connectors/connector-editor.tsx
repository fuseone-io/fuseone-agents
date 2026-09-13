import {
  connectorEditorFor,
  type ConfigurableConnector,
  type ConnectorEditorProps,
} from "@/features/integrations/connectors/connector-editor-registry";

export function ConnectorEditor({
  connector,
  ...props
}: ConnectorEditorProps & { connector: ConfigurableConnector }) {
  const definition = connectorEditorFor(connector);
  if (!definition) return null;
  const Editor = definition.Editor;
  return <Editor {...props} />;
}
