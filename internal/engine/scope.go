package engine

// placed is p with where its tool belongs, read once from the catalogue. Every
// proposal that reaches decide passes through here, so the Gate rules on the
// scope the ledger then records (NT-019).
func (r *Runner) placed(p Proposal) Proposal {
	p.toolScope, _ = r.deps.Catalog.Scope(p.Tool)
	return p
}
