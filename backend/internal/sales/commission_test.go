package sales

import "testing"

func TestSplitCommission(t *testing.T) {
	cases := []struct {
		total         int64
		bps           int
		store, vendor int64
	}{
		{10000, 2000, 2000, 8000},
		{9999, 1500, 1500, 8499},
		{3333, 3333, 1111, 2222},
		{5000, 10000, 5000, 0},
		{5000, 0, 0, 5000},
	}
	for _, c := range cases {
		store, vendor := splitCommission(c.total, c.bps)
		if store != c.store || vendor != c.vendor {
			t.Errorf("splitCommission(%d, %d) = %d, %d; want %d, %d", c.total, c.bps, store, vendor, c.store, c.vendor)
		}
	}
}
