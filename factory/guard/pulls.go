package guard

import (
	"context"
	"strings"
)

// pullRequest holds the PR fields a decision reads. Title, body, labels and
// every other text field are left undecoded on purpose: what is never read
// cannot leak into a result.
type pullRequest struct {
	State     string `json:"state"`
	Merged    bool   `json:"merged"`
	Mergeable *bool  `json:"mergeable"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

func (c *Client) pullRequest(ctx context.Context, number int) (pullRequest, error) {
	var pr pullRequest
	err := c.get(ctx, c.repoPath("/pulls/%d", number), &pr)
	return pr, err
}

// target is what a PR must be aimed at before either tool acts on it.
type target struct {
	repo Repo
	base string
}

// checkPullRequest refuses a PR outside the one repository and branch the
// ruleset protects (#207), a PR that is no longer open, and a head that is no
// longer the one the caller judged.
func checkPullRequest(want target, pr pullRequest, headSHA string) Reason {
	switch {
	case !strings.EqualFold(pr.Base.Repo.FullName, want.repo.FullName()):
		return ReasonWrongRepository
	case pr.Base.Ref != want.base:
		return ReasonWrongBase
	case pr.State != "open" || pr.Merged:
		return ReasonPRNotOpen
	case pr.Head.SHA != headSHA:
		return ReasonHeadMoved
	}
	return ""
}
