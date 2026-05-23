package assetbatch

const MaxDecodedBatchBody = 64 << 20

func WireReadLimit() int64 {
	return int64(MaxDecodedBatchBody + 1)
}
