package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

var removalInstant = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func removePerson(t *testing.T, people *fakePeople, caller domain.Role) openapi.RemovePersonResponseObject {
	t.Helper()
	resp, err := peopleServer(people).WithClock(fixedClock(removalInstant)).
		RemovePerson(asInstallation(caller), openapi.RemovePersonRequestObject{PrincipalId: "usr_bia"})
	if err != nil {
		t.Fatalf("RemovePerson: %v", err)
	}
	return resp
}

func TestRemovePerson_removesWhoTheCallerNamedAsTheCallerAtTheClocksInstant(t *testing.T) {
	t.Parallel()
	people := &fakePeople{}

	resp := removePerson(t, people, domain.RoleAdmin)

	if _, ok := resp.(openapi.RemovePerson204Response); !ok {
		t.Fatalf("resp = %T, want 204", resp)
	}
	want := auth.Removal{PrincipalID: "usr_bia", By: "usr_ana", At: removalInstant}
	if people.removed != want {
		t.Errorf("removal = %+v, want %+v", people.removed, want)
	}
}

func TestRemovePerson_withoutIdentityWrite_isForbiddenAndRemovesNobody(t *testing.T) {
	t.Parallel()
	people := &fakePeople{}

	resp := removePerson(t, people, domain.RoleAuthor)

	if _, ok := resp.(openapi.RemovePerson403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("resp = %T, want 403", resp)
	}
	if people.removed != (auth.Removal{}) {
		t.Errorf("removed %+v without the permission", people.removed)
	}
}

func TestRemovePerson_refusals_answerTheirCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		refuse error
		status int
		code   Code
	}{
		{auth.ErrNoPrincipal, http.StatusNotFound, CodeNotFound},
		{auth.ErrNotLocal, http.StatusConflict, CodePersonNotLocal},
		{auth.ErrRemovingYourself, http.StatusConflict, CodePersonIsYou},
		{auth.ErrLastAdministrator, http.StatusConflict, CodeLastAdministrator},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			resp := removePerson(t, &fakePeople{refuse: tc.refuse}, domain.RoleAdmin)
			problem := problemOfRemoval(t, resp)
			if problem.Status != tc.status || problem.Type == nil || *problem.Type != string(tc.code) {
				t.Errorf("problem = %+v, want %d %s", problem, tc.status, tc.code)
			}
		})
	}
}

func problemOfRemoval(t *testing.T, resp openapi.RemovePersonResponseObject) openapi.Problem {
	t.Helper()
	switch r := resp.(type) {
	case openapi.RemovePerson404ApplicationProblemPlusJSONResponse:
		return openapi.Problem(r.NotFoundApplicationProblemPlusJSONResponse)
	case openapi.RemovePerson409ApplicationProblemPlusJSONResponse:
		return openapi.Problem(r)
	}
	t.Fatalf("resp = %T, want a refusal", resp)
	return openapi.Problem{}
}

// Wired as production wires it, with no clock replaced: the wall clock stands in.
func TestRemovePerson_withNoClockWired_usesTheWallClock(t *testing.T) {
	t.Parallel()
	people := &fakePeople{}

	_, err := peopleServer(people).RemovePerson(asInstallation(domain.RoleAdmin),
		openapi.RemovePersonRequestObject{PrincipalId: "usr_bia"})
	if err != nil {
		t.Fatalf("RemovePerson: %v", err)
	}
	if people.removed.At.IsZero() {
		t.Error("removed at no instant")
	}
}
