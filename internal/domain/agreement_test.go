package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// Promotion and demotion are deliberately asymmetric, and every assertion here
// is about that asymmetry being right.

func TestSuggestsPromotion_needsEnoughDecisionsToMeanAnything(t *testing.T) {
	t.Parallel()

	// Three out of three is not evidence. Suggesting promotion on it trains
	// people to dismiss the suggestion, which costs more than never making it.
	perfect := domain.Agreement{Approved: 3}
	if perfect.SuggestsPromotion() {
		t.Error("promotion suggested on three decisions")
	}

	enough := domain.Agreement{Approved: 20}
	if !enough.SuggestsPromotion() {
		t.Error("promotion not suggested after twenty unbroken agreements")
	}
}

func TestWarrantsDemotion_actsOnFarLessEvidence(t *testing.T) {
	t.Parallel()

	// The asymmetry: loosening on thin evidence risks harm, tightening on thin
	// evidence costs somebody a few clicks. An agent being overruled is doing
	// damage between somebody noticing and somebody acting.
	overruled := domain.Agreement{Approved: 2, Refused: 3}
	if !overruled.WarrantsDemotion() {
		t.Error("an agent refused three times in five is still acting alone")
	}
	if overruled.Decided() >= domain.PromoteAfter {
		t.Error("this test no longer demonstrates the asymmetry")
	}
}

func TestAgreement_nobodyAsked_isNotAgreement(t *testing.T) {
	t.Parallel()

	// A run nobody was asked about says nothing about whether they would have
	// agreed. Reading silence as consent is how an agent gets promoted for
	// never having been checked.
	quiet := domain.Agreement{}
	if quiet.Rate() != 0 {
		t.Errorf("rate = %v, want nothing claimed", quiet.Rate())
	}
	if quiet.SuggestsPromotion() || quiet.WarrantsDemotion() {
		t.Error("an agent nobody reviewed moved stage")
	}
}

func TestWarrantsDemotion_agreementJustUnderTheLine_stillActs(t *testing.T) {
	t.Parallel()

	// Four out of five is 80%, which is the line rather than under it. The
	// boundary is stated so a change to it is a decision rather than a drift.
	if (domain.Agreement{Approved: 4, Refused: 1}).WarrantsDemotion() {
		t.Error("demoted at exactly the threshold")
	}
	if !(domain.Agreement{Approved: 3, Refused: 2}).WarrantsDemotion() {
		t.Error("not demoted below the threshold")
	}
}
