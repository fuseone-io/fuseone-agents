package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// maxAgentFileBytes is the ceiling every request body already has in front of
// the router, so a file is never refused here that the server let through.
const maxAgentFileBytes = 2 << 20

/*
draftOf is the definition a publication carries, from the fields or from the
file, with the company the request names (NT-018, contract 9).

Not yet rendered: both formats go through the same canonical form after the
scope they name is authorised, so the same definition sent either way is one
version. A file is parsed here, before authorisation, because its scope is
inside it; what that parse can refuse is what the sender wrote, never anything
this installation holds.
*/
func draftOf(req openapi.PublishAgentRequestObject) (spec.Spec, openapi.PublishAgentResponseObject) {
	switch {
	case req.JSONBody != nil:
		draft, err := draftOfFields(req.AgentId, *req.JSONBody)
		if err != nil {
			return spec.Spec{}, publishInvalid(err.Error())
		}
		// The query names the company of a file. Sent beside the fields, it
		// has to say what the body says, or one request names two companies.
		if q := req.Params.Company; q != nil && domain.CompanyID(*q) != draft.Company {
			return spec.Spec{}, publishInvalid(fmt.Sprintf(
				"the query names company %q and the definition %q", *q, draft.Company))
		}
		return draft, nil
	case req.Body != nil:
		return draftOfFile(req)
	default:
		return spec.Spec{}, publishInvalid(
			"send the definition as application/json fields or as a text/markdown .agent.md file")
	}
}

// draftOfFile reads an .agent.md, bounded, and reconciles its id and company
// with the request's.
func draftOfFile(req openapi.PublishAgentRequestObject) (spec.Spec, openapi.PublishAgentResponseObject) {
	data, err := io.ReadAll(io.LimitReader(req.Body, maxAgentFileBytes+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || len(data) > maxAgentFileBytes {
		return spec.Spec{}, openapi.PublishAgent413ApplicationProblemPlusJSONResponse(refusal(
			http.StatusRequestEntityTooLarge, CodeInvalidInput, "Agent file too large",
			fmt.Sprintf("an agent file is at most %d bytes", maxAgentFileBytes)))
	}
	if err != nil {
		return spec.Spec{}, publishInvalid(fmt.Sprintf("read the agent file: %v", err))
	}
	draft, err := spec.Parse("agent.md", data)
	if err != nil {
		return spec.Spec{}, publishInvalid(err.Error())
	}
	if string(draft.ID) != req.AgentId {
		return spec.Spec{}, publishInvalid(fmt.Sprintf(
			"the file defines %q and the path publishes %q", draft.ID, req.AgentId))
	}
	company, detail := fileCompany(draft.Company, req.Params.Company)
	if detail != "" {
		return spec.Spec{}, publishInvalid(detail)
	}
	// Into the specification before it is rendered, so the file and the
	// fields that say the same thing render the same bytes.
	draft.Company = company
	return draft, nil
}

// fileCompany is the company a file is published into: its frontmatter's, or
// the query's for a file kept for several installations, never two.
func fileCompany(declared domain.CompanyID, query *string) (domain.CompanyID, string) {
	switch {
	case query == nil && declared == "":
		return "", "the agent's company is required: declare company in the frontmatter or send ?company="
	case query == nil:
		return declared, ""
	case declared != "" && domain.CompanyID(*query) != declared:
		return "", fmt.Sprintf("the query names company %q and the file %q", *query, declared)
	default:
		return domain.CompanyID(*query), ""
	}
}

func publishInvalid(detail string) openapi.PublishAgentResponseObject {
	return openapi.PublishAgent400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}
