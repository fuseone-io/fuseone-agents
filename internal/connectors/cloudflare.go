package connectors

var cloudflareConnector = Connector{
	ID:       "cloudflare",
	Name:     "Cloudflare block list",
	Category: CategorySecurity,
	Summary:  "Add attacker addresses to one configured Cloudflare IP list that a firewall rule blocks at the edge.",
	Maturity: MaturityRuntime,
	Guarantees: []string{
		"only the one configured list is ever written, never rules, zones or DNS",
		"private, reserved and instance-protected ranges are refused in code",
		"entries carry a machine-readable comment so an external job can expire them",
		"a daily ceiling bounds how many addresses one day may add",
	},
	Caveats: []string{
		"expiry is an external job's duty; the connector only writes the timestamped comment it prunes by",
		"promoting a block to permanent lives in the operator's infrastructure repository, not here",
	},
	Operations: []Operation{
		{
			ID:             "cloudflare.list_blocks",
			Name:           "List blocks",
			Summary:        "Reads the configured block list's entries with their comments and ages.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
		{
			ID:   "cloudflare.block_ip",
			Name: "Block IP",
			// Write, not destructive: the catalog reserves destructive for
			// what cannot be undone, and this block is reversible twice over
			// — an external job expires it and removing the entry restores
			// the address. Policy still decides whether a human approves it,
			// because the use case this exists for is the night shift no
			// human is awake for.
			Summary:        "Adds one literal IP address to the configured block list with an expiry comment.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
	},
}
