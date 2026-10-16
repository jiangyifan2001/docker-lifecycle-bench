package monotime

type Clock interface {
	NowNS() (uint64, error)
}

type RawClock struct{}
