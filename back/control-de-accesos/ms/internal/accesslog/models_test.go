package accesslog

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func validReq() CreateRequest {
	return CreateRequest{
		ClientID: uuid.New(), Movement: MovementEntry, Result: ResultAuthorized,
		VisitorName: "Juan Pérez", UnitLabel: "Torre 3, Apto 502",
	}
}

func TestCreateRequestValidate(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-2 * time.Hour)

	cases := []struct {
		name   string
		mutate func(*CreateRequest)
		field  string // "" = must be valid
	}{
		{"valid entry", func(r *CreateRequest) {}, ""},
		{"valid exit", func(r *CreateRequest) { r.Movement = MovementExit }, ""},
		{"valid denied attempt", func(r *CreateRequest) { r.Movement = MovementAttempt; r.Result = ResultDenied }, ""},
		{"offline backdated within window", func(r *CreateRequest) { r.OccurredAt = &recent }, ""},
		{"missing client_id", func(r *CreateRequest) { r.ClientID = uuid.Nil }, "client_id"},
		{"bad movement", func(r *CreateRequest) { r.Movement = "IN" }, "movement"},
		{"bad result", func(r *CreateRequest) { r.Result = "OK" }, "result"},
		{"authorized attempt", func(r *CreateRequest) { r.Movement = MovementAttempt }, "result"},
		{"denied entry", func(r *CreateRequest) { r.Result = ResultDenied }, "result"},
		{"blank visitor", func(r *CreateRequest) { r.VisitorName = "  " }, "visitor_name"},
		{"blank unit", func(r *CreateRequest) { r.UnitLabel = "" }, "unit_label"},
		{"invalid utf-8", func(r *CreateRequest) { r.VisitorName = "Juan P\xe9rez" }, "visitor_name"},
		{"future timestamp", func(r *CreateRequest) { r.OccurredAt = &future }, "occurred_at"},
		{"too old timestamp", func(r *CreateRequest) { r.OccurredAt = &old }, "occurred_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReq()
			tc.mutate(&r)
			errs := r.validate(now)
			if tc.field == "" && len(errs) > 0 {
				t.Fatalf("expected valid, got %v", errs)
			}
			if tc.field != "" && len(errs[tc.field]) == 0 {
				t.Fatalf("expected error on %q, got %v", tc.field, errs)
			}
		})
	}
}

func TestListFilterValidate(t *testing.T) {
	a := time.Now()
	b := a.Add(time.Hour)
	if errs := (ListFilter{From: &a, To: &b, Movement: MovementExit}).validate(); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if errs := (ListFilter{From: &b, To: &a}).validate(); len(errs["to"]) == 0 {
		t.Fatal("expected error for to <= from")
	}
	if errs := (ListFilter{Movement: "X"}).validate(); len(errs["movement"]) == 0 {
		t.Fatal("expected error for bad movement")
	}
}
