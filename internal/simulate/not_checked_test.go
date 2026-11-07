package simulate

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestNotChecked_namesWhatThePinnedVersionDidNotHave(t *testing.T) {
	t.Parallel()
	working := []domain.RegressionCase{{ID: "velho"}, {ID: "novo"}, {ID: "perdido", Erased: true}}
	cases := []struct {
		name   string
		report Report
		want   []string
	}{
		{
			"a case added after the battery",
			Report{DatasetVersion: 1, Cases: []Case{{ID: "velho"}, {ID: "perdido", Settled: SettledErased}}},
			[]string{"novo"},
		},
		// Nothing can check a case with no input, and the report already calls it a gap.
		{
			"an erased case is not asked of anybody",
			Report{DatasetVersion: 1, Cases: []Case{{ID: "velho"}, {ID: "novo"}}},
			nil,
		},
		{
			"every case in the battery",
			Report{DatasetVersion: 1, Cases: []Case{{ID: "velho"}, {ID: "novo"}, {ID: "perdido"}}},
			nil,
		},
		// Read against the corpus as it is, a case that did not run is already broken.
		{
			"no pin says nothing",
			Report{Cases: []Case{{ID: "velho"}}},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NotChecked(tc.report, working); !slices.Equal(got, tc.want) {
				t.Errorf("not checked = %v, want %v", got, tc.want)
			}
		})
	}
}
