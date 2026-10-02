package accesslog

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCheckTransition(t *testing.T) {
	cases := []struct {
		name       string
		last, want string
		err        error
	}{
		{"first movement can be entry", "", MovementEntry, nil},
		{"first movement can be exit", "", MovementExit, nil},
		{"entry after exit", MovementExit, MovementEntry, nil},
		{"exit after entry", MovementEntry, MovementExit, nil},
		{"entry while inside", MovementEntry, MovementEntry, ErrAlreadyInside},
		{"exit while outside", MovementExit, MovementExit, ErrAlreadyOutside},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkTransition(tc.last, tc.want); !errors.Is(err, tc.err) {
				t.Fatalf("checkTransition(%q, %q) = %v, want %v", tc.last, tc.want, err, tc.err)
			}
		})
	}
}

func TestStatusFrom(t *testing.T) {
	for last, want := range map[string]string{
		"":            StatusUnknown,
		MovementEntry: StatusInside,
		MovementExit:  StatusOutside,
	} {
		if got := statusFrom(last); got != want {
			t.Errorf("statusFrom(%q) = %q, want %q", last, got, want)
		}
	}
}

func TestResidentMovementRequestValidate(t *testing.T) {
	ok := ResidentMovementRequest{ClientID: uuid.New(), Movement: MovementEntry}
	if errs := ok.validate(); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if errs := (ResidentMovementRequest{Movement: MovementExit}).validate(); len(errs["client_id"]) == 0 {
		t.Fatal("expected error for missing client_id")
	}
	// A resident can't register an ATTEMPT or anything else: only ENTRY/EXIT.
	for _, m := range []string{"", MovementAttempt, "IN", "entry"} {
		if errs := (ResidentMovementRequest{ClientID: uuid.New(), Movement: m}).validate(); len(errs["movement"]) == 0 {
			t.Errorf("movement %q should be rejected", m)
		}
	}
}
