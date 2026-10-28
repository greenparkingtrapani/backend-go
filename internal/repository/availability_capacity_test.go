package repository

import "testing"

func TestSharedCapacityThreeFullMotorcyclesPerCarSpace(t *testing.T) {
	tests := []struct {
		name                string
		vehicle             string
		cars, motos         int
		wantTotal, wantUsed int
	}{
		{"zero motos", "car", 0, 0, 12, 0},
		{"one moto keeps 12 car spots", "car", 0, 1, 12, 0},
		{"two motos keep 12 car spots", "car", 0, 2, 12, 0},
		{"three motos remove one", "car", 0, 3, 12, 1},
		{"five motos remove one", "car", 0, 5, 12, 1},
		{"six motos remove two", "car", 0, 6, 12, 2},
		{"eight motos remove two", "car", 0, 8, 12, 2},
		{"cars and motos combined", "car", 9, 8, 12, 11},
		{"maximum eight motos", "motorcycle", 0, 0, 8, 0},
		{"two motos fit with twelve cars", "motorcycle", 12, 2, 2, 2},
		{"third moto needs a car space", "motorcycle", 12, 3, 2, 3},
		{"five motos fit with eleven cars", "motorcycle", 11, 5, 5, 5},
		{"sixth moto needs a second space", "motorcycle", 11, 5, 5, 5},
		{"eight motos fit with ten cars", "motorcycle", 10, 7, 8, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			total, used := effectiveSpaces(tc.vehicle, 12, 8, tc.cars, tc.motos)
			if total != tc.wantTotal || used != tc.wantUsed {
				t.Fatalf("got total=%d used=%d, want total=%d used=%d", total, used, tc.wantTotal, tc.wantUsed)
			}
		})
	}
}
