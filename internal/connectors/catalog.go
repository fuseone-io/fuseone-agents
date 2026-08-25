package connectors

import "slices"

var catalog = []Connector{
	vaultConnector,
	approvedJobsConnector,
	sqlConnector,
	objectStorageConnector,
	identityConnector,
	kubernetesConnector,
	dnsConnector,
	smtpConnector,
	governedHTTPConnector,
}

// Catalog returns a defensive copy so callers cannot mutate the shared shape.
func Catalog() []Connector {
	out := make([]Connector, len(catalog))
	for i, c := range catalog {
		out[i] = c
		out[i].Guarantees = slices.Clone(c.Guarantees)
		out[i].Caveats = slices.Clone(c.Caveats)
		out[i].Operations = make([]Operation, len(c.Operations))
		for j, op := range c.Operations {
			out[i].Operations[j] = op
			out[i].Operations[j].Effects = slices.Clone(op.Effects)
		}
	}
	return out
}
