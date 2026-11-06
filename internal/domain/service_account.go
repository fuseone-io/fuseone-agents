package domain

import "time"

// ServiceAccount is a machine that acts on this installation: a pipeline, a script, a
// Terraform run. It holds grants the way a person does, and it can never hold what
// Principal.barredFrom keeps for people.
type ServiceAccount struct {
	ID          string
	Name        string
	Description string
	Disabled    bool
	CreatedAt   time.Time
	Grants      []Grant
}

// ServiceToken is what is known about a credential once it has been issued. The
// secret is not here: it is shown once, and only its hash is kept.
type ServiceToken struct {
	ID         string
	Name       string
	CreatedBy  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}
