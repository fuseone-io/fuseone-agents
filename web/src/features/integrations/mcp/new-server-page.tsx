import { Navigate } from "react-router-dom";

/**
 * Connecting a tool server, with the catalogue in front of it.
 *
 * A page rather than a dialog, and the dialog is why. A grid of cards has no
 * business in a modal — it drew over the fields underneath — but the deeper
 * reason is that a catalogue is read before anything is decided, and reading
 * needs somewhere to stand.
 *
 * The order is the order the decisions happen in: see what is known, fill the
 * form from one of them or not, and accept what a local server is. Nothing
 * here brings a tool in or says what one does; both are further along, on the
 * server's own page, and neither follows from connecting.
 */
export function NewServerPage() {
  return <Navigate to="/integrations/mcp" replace />;
}
