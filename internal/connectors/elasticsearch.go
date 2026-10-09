package connectors

var elasticsearchConnector = Connector{
	ID:       "elasticsearch",
	Name:     "Governed Elasticsearch window",
	Category: CategoryData,
	Summary:  "Answer historical traffic questions through named queries over one fixed index pattern, with a fixed projection.",
	Maturity: MaturityRuntime,
	Guarantees: []string{
		"the index pattern and the field mapping are configuration; arguments never choose what to read",
		"queries are named shapes built by the platform — a model supplies values, never DSL",
		"results carry only the projected fields, labeled untrusted; raw documents never reach a run",
		"every window is capped by the instance's ceiling",
	},
	Caveats: []string{
		"speaks the 6.x/7.x query dialect; an 8.x cluster is untested",
		"it reads what the index holds — coverage is the ingestion pipeline's duty",
	},
	Operations: []Operation{
		{
			ID:             "elasticsearch.top_ips",
			Name:           "Top IPs",
			Summary:        "Aggregates the busiest client addresses on a path over a window, with per-day and per-status breakdowns.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
		{
			ID:             "elasticsearch.resource_history",
			Name:           "Resource history",
			Summary:        "Lists the recent requests to one exact path, projected to timestamp, address, status, method and latency.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
	},
}
