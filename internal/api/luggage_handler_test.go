package api

import (
	"testing"
	"time"
)

func TestLuggageQuote(t *testing.T) {
	start := time.Date(2026, 11, 1, 9, 44, 0, 0, time.UTC)
	req := LuggageRequest{DropoffAt: start, PickupAt: start.Add(2*time.Hour + 31*time.Minute), Bags: []LuggagePiece{{WeightKg: 8, LengthCm: 60, WidthCm: 40, HeightCm: 20}, {WeightKg: 15, LengthCm: 70, WidthCm: 50, HeightCm: 25}, {WeightKg: 25, LengthCm: 80, WidthCm: 50, HeightCm: 40}}}
	hours, cents, err := luggageQuote(req)
	if err != nil || hours != 3 || cents != 1800 {
		t.Fatalf("got %d hours, %d cents, %v", hours, cents, err)
	}
	req.PickupAt = req.DropoffAt
	if _, _, err := luggageQuote(req); err == nil {
		t.Fatal("expected invalid duration")
	}
	req.PickupAt = start.Add(time.Hour)
	req.Bags[0].WeightKg = 0
	if _, _, err := luggageQuote(req); err == nil {
		t.Fatal("expected invalid weight")
	}
}
