package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

type fakeAccounts struct {
	createdBy string
	created   string
	password  string
	username  string
}

func (f *fakeAccounts) Create(_ context.Context, username, _, _, _, by string) (string, error) {
	f.created, f.createdBy = username, by
	return "usr_new", nil
}

func (f *fakeAccounts) SetPassword(_ context.Context, principalID, password string) error {
	f.created, f.password = principalID, password
	return nil
}

func (f *fakeAccounts) SetUsername(_ context.Context, principalID, username string) error {
	f.created, f.username = principalID, username
	return nil
}

func accountsServer(accounts *fakeAccounts) *Server {
	return NewServer(ledger.NewMemory(), "test").WithAccounts(accounts)
}

func localPersonRequest() openapi.CreateLocalPersonRequestObject {
	return openapi.CreateLocalPersonRequestObject{
		Body: &openapi.CreateLocalPersonJSONRequestBody{
			Username: "ana",
			Password: "secret",
		},
	}
}

func passwordRequest(principal string) openapi.SetPasswordRequestObject {
	return openapi.SetPasswordRequestObject{
		PrincipalId: principal,
		Body: &openapi.SetPasswordJSONRequestBody{
			Password: "new-secret",
		},
	}
}

func TestCreateLocalPerson_requiresInstallationIdentityWrite(t *testing.T) {
	t.Parallel()

	accounts := &fakeAccounts{}
	resp, err := accountsServer(accounts).CreateLocalPerson(as(domain.RoleAdmin), localPersonRequest())
	if err != nil {
		t.Fatalf("CreateLocalPerson: %v", err)
	}
	if _, ok := resp.(openapi.CreateLocalPerson403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if accounts.created != "" {
		t.Error("an area administrator created a local account")
	}
}

func TestCreateLocalPerson_withInstallationIdentityWriteCreatesTheAccount(t *testing.T) {
	t.Parallel()

	accounts := &fakeAccounts{}
	resp, err := accountsServer(accounts).CreateLocalPerson(asInstallation(domain.RoleAdmin), localPersonRequest())
	if err != nil {
		t.Fatalf("CreateLocalPerson: %v", err)
	}
	if _, ok := resp.(openapi.CreateLocalPerson201JSONResponse); !ok {
		t.Fatalf("response = %T, want 201", resp)
	}
	if accounts.created != "ana" || accounts.createdBy == "" {
		t.Errorf("created = %q by %q", accounts.created, accounts.createdBy)
	}
}

func TestSetPasswordForAnotherPerson_requiresInstallationIdentityWrite(t *testing.T) {
	t.Parallel()

	accounts := &fakeAccounts{}
	resp, err := accountsServer(accounts).SetPassword(as(domain.RoleAdmin), passwordRequest("usr_other"))
	if err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, ok := resp.(openapi.SetPassword403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if accounts.password != "" {
		t.Error("an area administrator changed somebody else's password")
	}
}

func TestSetPasswordForSelfDoesNotNeedIdentityWrite(t *testing.T) {
	t.Parallel()

	accounts := &fakeAccounts{}
	resp, err := accountsServer(accounts).SetPassword(as(domain.RoleAuthor), passwordRequest("usr_ana"))
	if err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, ok := resp.(openapi.SetPassword204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if accounts.password != "new-secret" {
		t.Errorf("password = %q", accounts.password)
	}
}
