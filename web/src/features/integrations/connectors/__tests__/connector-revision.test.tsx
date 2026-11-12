import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { ConnectorEditor } from "@/features/integrations/connectors/connector-editor";
import type { ConfigurableConnector } from "@/features/integrations/connectors/connector-editor-registry";
import { ConnectorInstancesPanel } from "@/features/integrations/connectors/connector-instances-panel";
import {
  useDeleteConnectorInstance,
  usePutConnectorInstance,
  type ConnectorInstance,
} from "@/features/integrations/api";
import { setLocale } from "@/i18n";
import { revisedCollection, stubNetwork } from "@/test/revised-routes";

const vault: ConnectorInstance = {
  name: "prod", connector: "vault", enabled: true, scopeKind: "installation", hasToken: true,
  editable: true,
  vault: { address: "https://vault.example.com/a", mount: "kv", allowedPathPrefixes: ["apps/"] },
};

const apim: ConnectorInstance = {
  name: "apim", connector: "gravitee", enabled: true, scopeKind: "area", company: "acme", area: "platform",
  hasToken: false,
  editable: true,
  gravitee: {
    address: "https://apim.example.com/management/v2", organization: "org", environment: "env",
    allowedReferences: [{ type: "API", id: "checkout-api" }], minTTLSeconds: 86_400, maxTTLSeconds: 7_776_000,
    allowNoExpiry: false,
    credentialSource: { kind: "vault_kv_secret", vaultInstance: "prod", path: "apps/gravitee", field: "token" },
  },
} as ConnectorInstance;

function world() {
  const instances = revisedCollection("/admin/integrations/connectors/instances", [vault, apim]);
  const requests = stubNetwork([instances], () => undefined);
  return { instances, writes: (method: string) => requests.filter((r) => r.method === method) };
}

function Editing({ instance = vault }: { instance?: ConnectorInstance }) {
  const put = usePutConnectorInstance();
  return (
    <ConnectorEditor connector={instance.connector as ConfigurableConnector} instance={instance} instances={[vault, apim]}
      onClose={vi.fn()} onSave={async (input) => void (await put.mutateAsync(input))} />
  );
}

function Listing() {
  const remove = useDeleteConnectorInstance();
  return (
    <ConnectorInstancesPanel
      view={{ instances: [vault], isLoading: false, error: null, canConfigure: true }}
      actions={{ retry: vi.fn(), create: vi.fn(), edit: vi.fn(),
        remove: (instance, etag) => remove.mutate({ instance, etag }) }}
    />
  );
}

function show(node: ReactNode) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {node}
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("a connector instance written over what was read", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("is read by name when its editor opens, and saved under that revision", async () => {
    const net = world();
    show(<Editing />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-prod-1"');
  });

  it("keeps the edit when somebody wrote first, and reloads only when asked", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<Editing />);
    const mount = await screen.findByLabelText("Mount");
    await user.clear(mount);
    await user.type(mount, "mine");
    net.instances.elsewhere("prod", { vault: { ...vault.vault!, mount: "theirs" } });
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Somebody changed this while you were editing")).toBeVisible();
    expect(screen.getByLabelText("Mount")).toHaveValue("mine");
    await user.click(screen.getByRole("button", { name: "Reload what is stored" }));
    await waitFor(() => expect(screen.getByLabelText("Mount")).toHaveValue("theirs"));
  });

  it("is removed under the revision read when its confirmation opened", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<Listing />);
    net.instances.elsewhere("prod", {});
    await user.click(screen.getByRole("button", { name: "Remove instance" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Remove" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(net.writes("DELETE")).toHaveLength(1));
    expect(net.writes("DELETE")[0]?.ifMatch).toBe('"v1-prod-2"');
  });

  // An editor that reads the detail itself saves under the revision of that read.
  it("saves a detail-reading editor under the revision of its own read", async () => {
    const net = world();
    show(<Editing instance={apim} />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-apim-1"');
  });

  it("keeps a detail-reading editor open when somebody wrote first", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<Editing instance={apim} />);
    const save = await screen.findByRole("button", { name: "Save" });
    net.instances.elsewhere("apim", {});
    await user.click(save);

    expect(await screen.findByText("Somebody changed this while you were editing")).toBeVisible();
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-apim-1"');
  });
});
