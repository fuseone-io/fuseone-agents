package guard

import (
	"regexp"
	"strconv"
	"strings"
)

// BoardConfig pins the board tools to one project and its fields. Every id
// comes from configuration; the code knows only the closed set of names.
type BoardConfig struct {
	ProjectID       string
	StatusFieldID   string
	Lanes           map[Lane]string // lane -> Status option id
	WorkTypeFieldID string
	WorkTypes       map[WorkType]string
	SizeFieldID     string
	Sizes           map[Size]string
}

// laneOf maps a Status option id back to its lane. An option the
// configuration does not name is no lane at all.
func (b BoardConfig) laneOf(optionID string) (Lane, bool) {
	return reverse(b.Lanes, optionID)
}

func reverse[K ~string](m map[K]string, optionID string) (K, bool) {
	for k, id := range m {
		if optionID != "" && id == optionID {
			return k, true
		}
	}
	return "", false
}

var (
	projectIDPattern = regexp.MustCompile(`^PVT_[A-Za-z0-9_-]{1,64}$`)
	fieldIDPattern   = regexp.MustCompile(`^PVTSSF_[A-Za-z0-9_-]{1,64}$`)
	optionIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// board reads the project, its Status field and the five lanes, all required:
// every role moves cards. withFields adds Work type and Size, for the roles
// that hold set_card_fields.
func (e *envReader) board(withFields bool) BoardConfig {
	b := BoardConfig{
		ProjectID:     e.matching("FACTORY_PROJECT_ID", projectIDPattern),
		StatusFieldID: e.matching("FACTORY_STATUS_FIELD_ID", fieldIDPattern),
		Lanes:         options[Lane](e, "FACTORY_LANES", Lanes()),
	}
	if withFields {
		b.WorkTypeFieldID = e.matching("FACTORY_WORKTYPE_FIELD_ID", fieldIDPattern)
		b.WorkTypes = options[WorkType](e, "FACTORY_WORKTYPES", WorkTypes())
		b.SizeFieldID = e.matching("FACTORY_SIZE_FIELD_ID", fieldIDPattern)
		b.Sizes = options[Size](e, "FACTORY_SIZES", Sizes())
	}
	return b
}

func (e *envReader) matching(name string, pattern *regexp.Regexp) string {
	v := e.required(name)
	if v != "" && !pattern.MatchString(v) {
		e.fail(name, "is not a well-formed id")
	}
	return v
}

// options reads a complete name=option-id table for a closed set of names.
func options[K ~string](e *envReader, name string, set []K) map[K]string {
	out := map[K]string{}
	for k, v := range e.pairs(name, names(set), true, optionIDPattern.MatchString) {
		out[K(k)] = v
	}
	return out
}

// roleBots reads role=bot-user-id pairs. One bot serves one role, so an
// identity maps to exactly one surface; not every role needs to be listed.
func (e *envReader) roleBots(name string) map[int64]Role {
	isID := func(v string) bool { n, err := strconv.ParseInt(v, 10, 64); return err == nil && n > 0 }
	out := map[int64]Role{}
	for role, raw := range e.pairs(name, names(Roles()), false, isID) {
		n, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		out[n] = Role(role)
	}
	return out
}
