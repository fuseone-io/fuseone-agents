package guard

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// dropReadyLabel removes factory:ready-to-merge. A remove answered 404 means
// the label is already gone: done.
func (c *Client) dropReadyLabel(ctx context.Context, pr int) (Reason, int) {
	code, err := c.send(ctx, apiRequest{
		Method: http.MethodDelete,
		Path:   c.repoPath("/issues/%d/labels/%s", pr, url.PathEscape(LabelReadyToMerge)),
	})
	if code == http.StatusNotFound {
		return "", 0
	}
	if r := labelReason(err, code, http.StatusOK); r != "" {
		return r, code
	}
	return "", 0
}

func labelReason(err error, code, want int) Reason {
	switch {
	case errors.Is(err, ErrNotSent):
		return ReasonLabelNotSent
	case err != nil:
		return ReasonOutcomeUnknown
	case code != want:
		return ReasonLabelRefused
	}
	return ""
}
