package guard

import "regexp"

// Lane is a value of the board's Status field. The names are a closed set in
// code; the option ids behind them come only from configuration.
type Lane string

const (
	LaneTriagem   Lane = "Triagem"
	LanePreparado Lane = "Preparado"
	LaneDoing     Lane = "Doing"
	LaneTravado   Lane = "Travado"
	LaneDone      Lane = "Done"
)

// Lanes is every lane, in the board's order.
func Lanes() []Lane { return []Lane{LaneTriagem, LanePreparado, LaneDoing, LaneTravado, LaneDone} }

// WorkType and Size are the values of the board's Work type and Size fields.
type (
	WorkType string
	Size     string
)

// WorkTypes and Sizes are the closed sets set_card_fields accepts.
func WorkTypes() []WorkType { return []WorkType{"bug", "feature", "chore", "docs"} }
func Sizes() []Size         { return []Size{"S", "M", "L"} }

// CardField names a field set_card_fields reports on, never a GitHub field id.
type CardField string

const (
	FieldWorkType CardField = "work_type"
	FieldSize     CardField = "size"
)

// itemPattern is a ProjectV2Item node id. The guard passes only an id it
// checked to GitHub, and only a checked id comes back in a result.
var itemPattern = regexp.MustCompile(`^PVTI_[A-Za-z0-9_-]{1,64}$`)

func validItemID(s string) bool { return itemPattern.MatchString(s) }

// member reports whether v is one of set, exactly: "travado" is not Travado.
func member[T ~string](set []T, v string) (T, bool) {
	for _, s := range set {
		if string(s) == v {
			return s, true
		}
	}
	return "", false
}
