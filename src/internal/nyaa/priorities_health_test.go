package nyaa

import "testing"

func TestHealthTier_FloorsTenAndFifty(t *testing.T) {
	cases := []struct {
		seeders string
		want    int
	}{
		{"9", 0},
		{"10", 1},
		{"49", 1},
		{"50", 2},
	}
	for _, tc := range cases {
		got := healthTier(TorrentResult{Seeders: tc.seeders})
		if got != tc.want {
			t.Errorf("healthTier(%s seeders) = %d, want %d", tc.seeders, got, tc.want)
		}
	}
}
