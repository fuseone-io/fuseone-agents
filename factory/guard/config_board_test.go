package guard

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

// Test ids. Shaped like GitHub's, but not the live board's: those belong in
// the README and the deployment, never in code.
const (
	testProjectID     = "PVT_testproject"
	otherProjectID    = "PVT_otherproject"
	testStatusField   = "PVTSSF_status"
	testWorkTypeField = "PVTSSF_worktype"
	testSizeField     = "PVTSSF_size"
)

func testBoardConfig() BoardConfig {
	return BoardConfig{
		ProjectID:     testProjectID,
		StatusFieldID: testStatusField,
		Lanes: map[Lane]string{
			LaneTriagem: "opt-triagem", LanePreparado: "opt-preparado", LaneDoing: "opt-doing",
			LaneTravado: "opt-travado", LaneDone: "opt-done",
		},
		WorkTypeFieldID: testWorkTypeField,
		WorkTypes:       map[WorkType]string{"bug": "wt-bug", "feature": "wt-feature", "chore": "wt-chore", "docs": "wt-docs"},
		SizeFieldID:     testSizeField,
		Sizes:           map[Size]string{"S": "sz-s", "M": "sz-m", "L": "sz-l"},
	}
}

func withBoard(env map[string]string) map[string]string {
	maps.Copy(env, map[string]string{
		"FACTORY_PROJECT_ID":      testProjectID,
		"FACTORY_STATUS_FIELD_ID": testStatusField,
		"FACTORY_LANES":           "Triagem=opt-triagem,Preparado=opt-preparado,Doing=opt-doing,Travado=opt-travado,Done=opt-done",
	})
	return env
}

func withFields(env map[string]string) map[string]string {
	maps.Copy(env, map[string]string{
		"FACTORY_WORKTYPE_FIELD_ID": testWorkTypeField,
		"FACTORY_WORKTYPES":         "bug=wt-bug,feature=wt-feature,chore=wt-chore,docs=wt-docs",
		"FACTORY_SIZE_FIELD_ID":     testSizeField,
		"FACTORY_SIZES":             "S=sz-s,M=sz-m,L=sz-l",
	})
	return env
}

// triageEnv is a Triage instance under its own App: the board and its fields,
// and nothing about pull requests.
func triageEnv() map[string]string {
	return withFields(withBoard(map[string]string{
		"FACTORY_ROLE":                "triage",
		"FACTORY_REPOSITORY":          "fuseone-io/fuseone-agents",
		"GITHUB_APP_ID":               "321",
		"GITHUB_APP_INSTALLATION_ID":  "654",
		"GITHUB_APP_PRIVATE_KEY_FILE": "/run/secrets/key.pem",
	}))
}

// passThroughEnv serves every role from the tokens the platform sends.
func passThroughEnv() map[string]string {
	env := withFields(withBoard(mergerEnv()))
	for _, k := range []string{"FACTORY_ROLE", "GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY_FILE"} {
		delete(env, k)
	}
	env["FACTORY_MCP_AUTH"] = "github-app-token"
	env["FACTORY_ROLE_BOTS"] = "triage=101,architect=102,developer=103,reviewer=104,qa=9001,merger=106"
	return env
}

func TestLoadConfig_board_parsesProjectFieldsAndLanes(t *testing.T) {
	t.Parallel()

	cfg, err := load(triageEnv())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := testBoardConfig()
	if cfg.Board.ProjectID != want.ProjectID || cfg.Board.StatusFieldID != want.StatusFieldID ||
		!maps.Equal(cfg.Board.Lanes, want.Lanes) || !maps.Equal(cfg.Board.WorkTypes, want.WorkTypes) ||
		!maps.Equal(cfg.Board.Sizes, want.Sizes) || cfg.Board.SizeFieldID != want.SizeFieldID {
		t.Fatalf("board = %+v", cfg.Board)
	}
	if cfg.GraphQLURL != "https://api.github.com/graphql" {
		t.Fatalf("graphql url default = %q", cfg.GraphQLURL)
	}
}

// A board role needs no base branch and no QA identity: those belong to the
// surfaces that act on pull requests.
func TestLoadConfig_boardRole_needsNoPullRequestSettings(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"triage", "architect", "developer", "reviewer"} {
		env := triageEnv()
		env["FACTORY_ROLE"] = role
		if _, err := load(env); err != nil {
			t.Fatalf("%s: %v", role, err)
		}
	}
}

// Every id the board tools write with has no default.
func TestLoadConfig_boardVariableMissing_refusedNamingIt(t *testing.T) {
	t.Parallel()

	for _, drop := range []string{
		"FACTORY_PROJECT_ID", "FACTORY_STATUS_FIELD_ID", "FACTORY_LANES",
		"FACTORY_WORKTYPE_FIELD_ID", "FACTORY_WORKTYPES", "FACTORY_SIZE_FIELD_ID", "FACTORY_SIZES",
	} {
		t.Run(drop, func(t *testing.T) {
			t.Parallel()
			env := triageEnv()
			delete(env, drop)
			if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), drop) {
				t.Fatalf("err = %v, want ErrConfig naming %s", err, drop)
			}
		})
	}
}

// Work type and Size are only for the roles that hold set_card_fields.
func TestLoadConfig_developer_needsNoCardFields(t *testing.T) {
	t.Parallel()

	env := withBoard(map[string]string{
		"FACTORY_ROLE": "developer", "FACTORY_REPOSITORY": "fuseone-io/fuseone-agents",
		"GITHUB_APP_ID": "1", "GITHUB_APP_INSTALLATION_ID": "2", "GITHUB_APP_PRIVATE_KEY_FILE": "/run/secrets/key.pem",
	})
	if _, err := load(env); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestLoadConfig_malformedTable_refused(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"partial lanes":     {"FACTORY_LANES", "Triagem=a,Preparado=b,Doing=c,Travado=d"},
		"duplicate id":      {"FACTORY_LANES", "Triagem=a,Preparado=a,Doing=c,Travado=d,Done=e"},
		"duplicate name":    {"FACTORY_LANES", "Triagem=a,Triagem=b,Doing=c,Travado=d,Done=e"},
		"unknown name":      {"FACTORY_LANES", "Triagem=a,Preparado=b,Doing=c,Travado=d,Backlog=e"},
		"lowercase name":    {"FACTORY_LANES", "triagem=a,Preparado=b,Doing=c,Travado=d,Done=e"},
		"empty option":      {"FACTORY_LANES", "Triagem=,Preparado=b,Doing=c,Travado=d,Done=e"},
		"no equals":         {"FACTORY_LANES", "Triagem,Preparado=b,Doing=c,Travado=d,Done=e"},
		"partial sizes":     {"FACTORY_SIZES", "S=a,M=b"},
		"unknown work type": {"FACTORY_WORKTYPES", "bug=a,feature=b,chore=c,docs=d,epic=e"},
		"project shape":     {"FACTORY_PROJECT_ID", "PVTSSF_notaproject"},
		"field shape":       {"FACTORY_STATUS_FIELD_ID", "PVT_notafield"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := triageEnv()
			env[tc[0]] = tc[1]
			if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), tc[0]) {
				t.Fatalf("err = %v, want ErrConfig naming %s", err, tc[0])
			}
		})
	}
}

// In pass-through mode the server holds no credential: GITHUB_APP_* is not
// read, even when present and broken.
func TestLoadConfig_passThrough_readsNoAppCredentials(t *testing.T) {
	t.Parallel()

	env := passThroughEnv()
	env["GITHUB_APP_ID"] = "not-a-number"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth != AuthPassThrough || cfg.App.AppID != 0 || cfg.App.PrivateKeyPEM != nil {
		t.Fatalf("auth %q, app %+v", cfg.Auth, cfg.App)
	}
	if cfg.RoleBots[101] != RoleTriage || cfg.RoleBots[106] != RoleMerger || len(cfg.RoleBots) != 6 {
		t.Fatalf("role bots = %v", cfg.RoleBots)
	}
}

func TestLoadConfig_passThroughAllowlist_malformed_refused(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"missing":        "",
		"unknown role":   "triage=1,curator=2",
		"duplicate bot":  "triage=1,architect=1",
		"duplicate role": "triage=1,triage=2",
		"login not id":   "triage=fuseone-triage[bot]",
		"negative id":    "triage=-3",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := passThroughEnv()
			env["FACTORY_ROLE_BOTS"] = value
			if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "FACTORY_ROLE_BOTS") {
				t.Fatalf("err = %v, want ErrConfig naming FACTORY_ROLE_BOTS", err)
			}
		})
	}
}

// FACTORY_ROLE would look like it narrows a pass-through server. It does not,
// so setting it is an error rather than a silent no-op.
func TestLoadConfig_passThroughWithRole_refused(t *testing.T) {
	t.Parallel()

	env := passThroughEnv()
	env["FACTORY_ROLE"] = "qa"
	if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "FACTORY_ROLE") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_ROLE", err)
	}
}

// Only the listed roles are served, and only their settings are required.
func TestLoadConfig_passThroughBoardOnly_needsNoPullRequestSettings(t *testing.T) {
	t.Parallel()

	env := withFields(withBoard(map[string]string{
		"FACTORY_MCP_AUTH": "github-app-token", "FACTORY_ROLE_BOTS": "triage=101,developer=103",
	}))
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Roles(); len(got) != 2 || got[0] != RoleTriage || got[1] != RoleDeveloper {
		t.Fatalf("roles = %v", got)
	}
}

func TestLoadConfig_unknownAuthMode_refused(t *testing.T) {
	t.Parallel()

	env := passThroughEnv()
	env["FACTORY_MCP_AUTH"] = "token"
	if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "FACTORY_MCP_AUTH") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_MCP_AUTH", err)
	}
}
