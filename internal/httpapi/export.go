package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/export"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Signing is the installation's export key, declared here by the consumer.
type Signing interface {
	Key(ctx context.Context) (ed25519.PrivateKey, error)
	PublicKey(ctx context.Context) (ed25519.PublicKey, error)
}

// WithSigning wires the key exports are signed with.
func (s *Server) WithSigning(signing Signing) *Server {
	s.signing = signing
	return s
}

// ExportLedger returns a signed range somebody outside can check.
func (s *Server) ExportLedger(
	ctx context.Context, req openapi.ExportLedgerRequestObject,
) (openapi.ExportLedgerResponseObject, error) {
	// Exporting is its own authority, held by the auditor and nobody else by
	// default: a range of the ledger is the whole record of what was done to
	// whom, leaving the installation in one file.
	if err := auth.Require(ctx, domain.PermAuditExport, adminScope); err != nil {
		return openapi.ExportLedger403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAuditExport, adminScope),
		}, nil
	}
	if s.signing == nil {
		return openapi.ExportLedger404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Params.RunId),
		}, nil
	}

	steps, err := s.store.Read(ctx, domain.RunID(req.Params.RunId), domain.FirstSeq)
	if err != nil || len(steps) == 0 {
		return openapi.ExportLedger404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Params.RunId),
		}, nil
	}

	key, err := s.signing.Key(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the signing key: %w", err)
	}
	bundle, err := export.Build(string(steps[0].Scope.Company), steps, key)
	if err != nil {
		return nil, fmt.Errorf("build export: %w", err)
	}

	// Rendered through the bundle's own encoder rather than the generated
	// type: the file an auditor verifies and the body this returns have to be
	// the same document, and two encoders is how they stop being.
	raw, err := bundle.Encode()
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("render export: %w", err)
	}
	return openapi.ExportLedger200JSONResponse(body), nil
}

// GetSigningKey publishes the public half.
func (s *Server) GetSigningKey(
	ctx context.Context, _ openapi.GetSigningKeyRequestObject,
) (openapi.GetSigningKeyResponseObject, error) {
	if s.signing == nil {
		return openapi.GetSigningKey200JSONResponse{}, nil
	}
	public, err := s.signing.PublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the signing key: %w", err)
	}

	return openapi.GetSigningKey200JSONResponse{
		PublicKey:   base64.StdEncoding.EncodeToString(public),
		Fingerprint: export.Bundle{PublicKey: public}.Fingerprint(),
	}, nil
}
