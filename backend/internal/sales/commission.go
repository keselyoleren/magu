package sales

func splitCommission(lineTotal int64, bps int) (store, vendor int64) {
	store = (lineTotal*int64(bps) + 5000) / 10000
	return store, lineTotal - store
}
