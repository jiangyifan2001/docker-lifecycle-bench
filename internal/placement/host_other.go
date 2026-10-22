//go:build !linux

package placement

import (
	"context"
	"fmt"
	"time"
)

func PinHarness(cpu int) error {
	return fmt.Errorf("single-CPU mode requires Linux; cannot pin CPU %d on this OS", cpu)
}
func CheckHost(_ context.Context, cpu int, _ string) Snapshot {
	return Snapshot{CPU: cpu, CollectedAt: time.Now().UTC(), Error: "single-CPU mode requires Linux"}
}
