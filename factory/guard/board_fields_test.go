package guard

import (
	"slices"
	"testing"
)

func (h *boardHarness) setFields(wt, size string) FieldsResult {
	h.t.Helper()
	res, err := h.board(RoleTriage).SetFields(h.t.Context(), FieldsInput{ItemID: testItem, WorkType: wt, Size: size})
	if err != nil {
		h.t.Fatalf("set_card_fields returned an error: %v", err)
	}
	return res
}

func TestSetCardFields_validValues_setBothWithTheConfiguredFields(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	res := h.setFields("feature", "M")
	if !res.Updated || res.Reason != ReasonUpdated || res.WorkType != "feature" || res.Size != "M" ||
		!slices.Equal(res.Changed, []CardField{FieldWorkType, FieldSize}) || len(res.AlreadySet) != 0 {
		t.Fatalf("result = %+v", res)
	}
	want := []fakeMutation{
		{Token: roleToken(RoleTriage), Project: testProjectID, Item: testItem, Field: testWorkTypeField, Option: "wt-feature"},
		{Token: roleToken(RoleTriage), Project: testProjectID, Item: testItem, Field: testSizeField, Option: "sz-m"},
	}
	if got := h.mutations(); !slices.Equal(got, want) {
		t.Fatalf("mutations = %+v, want %+v", got, want)
	}
}

// Work type and Size are set on the way out of Triagem. A card in any other
// lane is refused and nothing is sent: in Preparado and Doing they are
// locked, and Travado and Done belong to a person. The values asked for are
// ones the card does not hold, so a tool that let the call through would
// write.
func TestSetCardFields_cardOutsideTriagem_refusedWithoutMutation(t *testing.T) {
	t.Parallel()

	for lane, want := range map[Lane]Reason{
		LanePreparado: ReasonFieldsLocked, LaneDoing: ReasonFieldsLocked,
		LaneTravado: ReasonTravadoIsHuman, LaneDone: ReasonDoneIsFinal,
	} {
		t.Run(string(lane), func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, lane)
			res := h.setFields("bug", "S")
			if res.Updated || res.Reason != want || len(h.mutations()) != 0 {
				t.Fatalf("result = %+v after %d mutations, want %s and none", res, len(h.mutations()), want)
			}
		})
	}
}

func TestSetCardFields_cardRefusals_matchMoveCard(t *testing.T) {
	t.Parallel()

	cases := map[Reason]func(f *fakeGraphQL){
		ReasonWrongProject: func(f *fakeGraphQL) { f.items[testItem].ProjectID = otherProjectID },
		ReasonItemArchived: func(f *fakeGraphQL) { f.items[testItem].Archived = true },
		ReasonItemNotFound: func(f *fakeGraphQL) { delete(f.items, testItem) },
		ReasonLaneUnknown:  func(f *fakeGraphQL) { f.items[testItem].Status = "" },
	}
	for want, change := range cases {
		t.Run(string(want), func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			h.edit(change)
			if res := h.setFields("bug", "S"); res.Updated || res.Reason != want || len(h.mutations()) != 0 {
				t.Fatalf("result = %+v after %d mutations, want %s and none", res, len(h.mutations()), want)
			}
		})
	}
}

func TestSetCardFields_valuesOutsideTheClosedSets_invalidInput(t *testing.T) {
	t.Parallel()

	for _, tc := range [][2]string{{"epic", "S"}, {"Bug", "S"}, {"bug", "s"}, {"bug", "XL"}, {"", "M"}, {"wt-bug", "sz-s"}} {
		h := newBoardHarness(t, LaneTriagem)
		if res := h.setFields(tc[0], tc[1]); res.Reason != ReasonInvalidInput || len(h.mutations()) != 0 {
			t.Fatalf("%v: result = %+v", tc, res)
		}
	}
}

// Work type was set, Size was refused. The retry writes only Size, and says
// which field was already set and which it changed.
func TestSetCardFields_partialFailureThenRetry_converges(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	h.edit(func(f *fakeGraphQL) { f.refuseField = testSizeField })
	first := h.setFields("bug", "L")
	if first.Updated || first.Reason != ReasonUpdateRefused || !slices.Equal(first.Changed, []CardField{FieldWorkType}) {
		t.Fatalf("first = %+v, want update_refused having changed work_type only", first)
	}
	h.edit(func(f *fakeGraphQL) { f.refuseField = "" })
	retry := h.setFields("bug", "L")
	if !retry.Updated || retry.Reason != ReasonUpdated ||
		!slices.Equal(retry.AlreadySet, []CardField{FieldWorkType}) || !slices.Equal(retry.Changed, []CardField{FieldSize}) {
		t.Fatalf("retry = %+v, want work_type already set and size changed", retry)
	}
	if got := h.mutations(); len(got) != 3 || got[2].Field != testSizeField {
		t.Fatalf("mutations = %+v, want the retry to write only Size", got)
	}
}

func TestSetCardFields_bothAlreadySet_successWithoutMutation(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	h.edit(func(f *fakeGraphQL) { f.items[testItem].WorkType, f.items[testItem].Size = "wt-docs", "sz-s" })
	res := h.setFields("docs", "S")
	if res.Updated || res.Reason != ReasonAlreadySet || len(res.AlreadySet) != 2 || len(h.mutations()) != 0 {
		t.Fatalf("result = %+v after %d mutations", res, len(h.mutations()))
	}
}
