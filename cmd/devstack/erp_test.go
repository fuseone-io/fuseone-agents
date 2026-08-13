package main

import (
	"testing"
)

/*
The fixture has to be worth trusting, or the lab teaches the wrong lesson.

Two properties matter. A refund actually moves the money back — a stand-in that
reported success and changed nothing would prove an undo reached a server and
nothing about whether it undid anything. And refunding twice moves it once: the
compensation sweep runs again after a crash, and a fixture that double-refunded
would demonstrate a bug the platform does not have.
*/
func TestRefund_afterATransfer_putsTheMoneyBack(t *testing.T) {
	t.Parallel()
	e := newERP()

	_, opened, err := e.balance(t.Context(), nil, balanceIn{Account: "acct_4471"})
	if err != nil {
		t.Fatalf("balance: %v", err)
	}

	_, made, err := e.transfer(t.Context(), nil,
		transferIn{From: "acct_4471", To: "acct_9002", Cents: 25_000})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	_, moved, _ := e.balance(t.Context(), nil, balanceIn{Account: "acct_4471"})
	if moved.Cents != opened.Cents-25_000 {
		t.Fatalf("balance after the transfer = %d, want %d", moved.Cents, opened.Cents-25_000)
	}

	if _, _, err := e.refund(t.Context(), nil, refundIn{TransferID: made.TransferID}); err != nil {
		t.Fatalf("refund: %v", err)
	}

	_, back, _ := e.balance(t.Context(), nil, balanceIn{Account: "acct_4471"})
	if back.Cents != opened.Cents {
		t.Errorf("balance after the refund = %d, want it back at %d", back.Cents, opened.Cents)
	}
}

func TestRefund_twice_movesTheMoneyOnce(t *testing.T) {
	t.Parallel()
	e := newERP()

	_, made, _ := e.transfer(t.Context(), nil,
		transferIn{From: "acct_4471", To: "acct_9002", Cents: 10_000})
	_, opened, _ := e.balance(t.Context(), nil, balanceIn{Account: "acct_4471"})

	for range 3 {
		if _, _, err := e.refund(t.Context(), nil, refundIn{TransferID: made.TransferID}); err != nil {
			t.Fatalf("refund: %v", err)
		}
	}

	_, after, _ := e.balance(t.Context(), nil, balanceIn{Account: "acct_4471"})
	if after.Cents != opened.Cents+10_000 {
		t.Errorf("balance = %d after three refunds, want one reversal", after.Cents)
	}
}

// The undo is called with what the do returned, so the two schemas have to
// line up. If they ever drift, the compensation reaches the server and is
// refused for a missing field — which reads as a broken platform.
func TestTransferOut_isRefundIn(t *testing.T) {
	t.Parallel()
	e := newERP()

	_, made, _ := e.transfer(t.Context(), nil,
		transferIn{From: "acct_4471", To: "acct_9002", Cents: 1})
	if _, _, err := e.refund(t.Context(), nil, refundIn{TransferID: made.TransferID}); err != nil {
		t.Fatalf("the undo could not read what the do answered: %v", err)
	}
}
