package guard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// The board's two GraphQL documents. Ids travel only as variables, never
// spliced into a document, and each asks only for what a decision reads:
// no title, no content, no text field.
const (
	itemQuery = `query($id: ID!) { node(id: $id) { ... on ProjectV2Item { id isArchived project { id } ` +
		`fieldValues(first: 50) { pageInfo { hasNextPage } nodes { ... on ProjectV2ItemFieldSingleSelectValue ` +
		`{ optionId field { ... on ProjectV2SingleSelectField { id } } } } } } } }`
	updateMutation = `mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) { ` +
		`updateProjectV2ItemFieldValue(input: {projectId: $project, itemId: $item, fieldId: $field, ` +
		`value: {singleSelectOptionId: $option}}) { projectV2Item { id } } }`
)

type graphQLBody struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables,omitempty"`
}

// graphQLError decodes the error's machine code and nothing else: the
// message is free text, and is never read.
type graphQLError struct {
	Type string `json:"type"`
}

func (c *Client) graphQL(ctx context.Context, body graphQLBody, out any) (int, error) {
	return c.send(ctx, apiRequest{
		Method: http.MethodPost, Path: "graphql", URL: c.opts.GraphQLURL, Body: body, Out: out,
	})
}

// projectItem is what the board tools decide with: whether the node is a
// project item at all, its project, whether it is archived, and the option
// id of each single-select field, keyed by field id.
type projectItem struct {
	Found     bool
	ProjectID string
	Archived  bool
	Options   map[string]string
}

type itemNode struct {
	ID          string              `json:"id"`
	IsArchived  bool                `json:"isArchived"`
	Project     struct{ ID string } `json:"project"`
	FieldValues struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []struct {
			OptionID string              `json:"optionId"`
			Field    struct{ ID string } `json:"field"`
		} `json:"nodes"`
	} `json:"fieldValues"`
}

// readItem reads one item. An id GitHub does not know, or one that is not a
// project item, is not found; any other GraphQL error fails the read.
func (c *Client) readItem(ctx context.Context, id string) (projectItem, error) {
	var out struct {
		Data struct {
			Node *itemNode `json:"node"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	code, err := c.graphQL(ctx, graphQLBody{Query: itemQuery, Variables: map[string]string{"id": id}}, &out)
	if err != nil {
		return projectItem{}, err
	}
	node := out.Data.Node
	switch {
	case code != http.StatusOK:
		return projectItem{}, fmt.Errorf("%w: POST graphql: item read: status %d", ErrGitHub, code)
	case node == nil && onlyNotFound(out.Errors):
		return projectItem{}, nil
	case len(out.Errors) > 0:
		return projectItem{}, fmt.Errorf("%w: POST graphql: item read: %d errors", ErrGitHub, len(out.Errors))
	case node == nil || node.ID != id:
		return projectItem{}, nil
	case node.FieldValues.PageInfo.HasNextPage:
		return projectItem{}, fmt.Errorf("%w: POST graphql: item read: more field values than one page", ErrGitHub)
	}
	item := projectItem{Found: true, ProjectID: node.Project.ID, Archived: node.IsArchived, Options: map[string]string{}}
	for _, v := range node.FieldValues.Nodes {
		if v.Field.ID != "" && v.OptionID != "" {
			item.Options[v.Field.ID] = v.OptionID
		}
	}
	return item, nil
}

func onlyNotFound(errs []graphQLError) bool {
	for _, e := range errs {
		if e.Type != "NOT_FOUND" {
			return false
		}
	}
	return true
}

// fieldUpdate is one single-select value to set. Every id in it comes from
// configuration, except the item's, which was validated and read.
type fieldUpdate struct {
	Project, Item, Field, Option string
}

// writeOutcome is what is known about an update after it was attempted.
type writeOutcome int

const (
	writeDone    writeOutcome = iota
	writeRefused              // GitHub answered, and did not set the value
	writeNotSent              // the request never left the process
	writeUnknown              // sent, and the answer was lost
)

// updateField sets one value with exactly one mutation. The answer counts as
// done only when it names the item back and carries no error.
func (c *Client) updateField(ctx context.Context, u fieldUpdate) (writeOutcome, int) {
	var out struct {
		Data struct {
			Update *struct {
				Item *struct{ ID string } `json:"projectV2Item"`
			} `json:"updateProjectV2ItemFieldValue"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	code, err := c.graphQL(ctx, graphQLBody{Query: updateMutation, Variables: map[string]string{
		"project": u.Project, "item": u.Item, "field": u.Field, "option": u.Option,
	}}, &out)
	switch {
	case errors.Is(err, ErrNotSent):
		return writeNotSent, 0
	case err != nil:
		return writeUnknown, 0
	case code != http.StatusOK:
		return writeRefused, code
	case len(out.Errors) > 0 || out.Data.Update == nil || out.Data.Update.Item == nil ||
		out.Data.Update.Item.ID != u.Item:
		return writeRefused, 0
	}
	return writeDone, 0
}
