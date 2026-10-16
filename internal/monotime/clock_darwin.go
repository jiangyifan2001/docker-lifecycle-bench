//go:build darwin

package monotime

import "golang.org/x/sys/unix"

func (RawClock) NowNS() (uint64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &ts); err != nil {
		return 0, err
	}
	return uint64(ts.Sec)*1_000_000_000 + uint64(ts.Nsec), nil
}
