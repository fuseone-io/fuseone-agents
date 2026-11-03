package main

import (
	"reflect"
	"testing"
)

// The Architect blocked the card and named Triagem; a person brought it
// back, Triage passed it to Preparado, and a person returned it. The lane it
// returned to is Triage's, but the card's last block was the Architect's.
func TestReturns_toTriagem_goToTheRoleOfTheLastBlockNotTheLane(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 40, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T08:00:00Z", "", "Triagem", person),
		comment(t, "2026-10-05T09:00:00Z", "Triagem", archBot),
		move(t, "2026-10-05T09:00:01Z", "Triagem", "Travado", archBot),
		comment(t, "2026-10-05T11:00:00Z", "Doing", person), // not a role's: ignored
		move(t, "2026-10-05T12:00:00Z", "Travado", "Triagem", person),
		move(t, "2026-10-05T13:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-06T08:00:00Z", "Preparado", "Triagem", person),
	}}
	want := []string{
		"architect: " + returnedKind("Travado", ruleReturnTo),
		"architect: " + returnedKind("Preparado", ruleReturnTo),
	}
	if got := signalsOf(t, snapshot(card)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReturns_withoutAReturnToComment_goToTheLastRoleWrite(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 41, OnBoard: true, Linked: []int{42}, Events: []Event{
		move(t, "2026-10-05T08:00:00Z", "", "Triagem", person),
		move(t, "2026-10-05T09:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-05T10:00:00Z", "Preparado", "Doing", person),
		move(t, "2026-10-06T10:00:00Z", "Doing", "Triagem", person),
	}}
	pr := &Item{Number: 42, IsPR: true, Author: devBot, Linked: []int{41}, Events: []Event{
		review(t, "2026-10-05T20:00:00Z", "COMMENTED", shaA, revBot),
		comment(t, "2026-10-06T11:00:00Z", "Doing", qaBot), // after the return
	}}
	want := []string{"reviewer: " + returnedKind("Doing", ruleLastWrite)}
	if got := signalsOf(t, snapshot(card, pr)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReturns_onlyAPersonsMoveBackIsAReturn(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 43, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T08:00:00Z", "", "Triagem", person),
		move(t, "2026-10-05T09:00:00Z", "Triagem", "Triagem", person),
		move(t, "2026-10-05T10:00:00Z", "Triagem", "Preparado", person),
		move(t, "2026-10-05T11:00:00Z", "Preparado", "Triagem", strayBot),
		move(t, "2026-10-05T12:00:00Z", "Triagem", "Preparado", person),
		move(t, "2026-10-05T13:00:00Z", "Preparado", "Triagem", person),
	}}
	want := []string{"unattributed: " + returnedKind("Preparado", ruleNoRole)}
	if got := signalsOf(t, snapshot(card)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
