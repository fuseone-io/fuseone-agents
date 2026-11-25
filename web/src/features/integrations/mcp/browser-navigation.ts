/**
 * Sends the browser to an address outside the console: the provider's
 * sign-in. The edge of the console a test stands in for, as the network is.
 */
export function sendBrowserTo(url: string) {
  window.location.assign(url);
}
