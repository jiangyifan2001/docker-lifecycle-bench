package main

import (
	"flag"
	"testing"

	"docker-lifecycle-bench/internal/campaign"
)

func TestCPUFlag(t *testing.T) {
	for _, test := range []struct {
		args    []string
		wantCPU int
		invalid bool
	}{
		{nil, -1, false}, {[]string{"--single-cpu", "0"}, 0, false},
		{[]string{"--single-cpu", "16"}, 16, false}, {[]string{"--single-cpu", "-1"}, -1, true},
		{[]string{"extra"}, -1, true},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Int("single-cpu", -1, "")
		if err := fs.Parse(test.args); err != nil {
			t.Fatal(err)
		}
		spec := campaign.Spec{}
		err := applyCPUFlag(fs, &spec)
		if (err != nil) != test.invalid {
			t.Fatalf("args=%v err=%v", test.args, err)
		}
		if test.invalid {
			continue
		}
		if test.wantCPU == -1 && spec.Placement != nil {
			t.Fatal("default unexpectedly pins")
		}
		if test.wantCPU >= 0 && (spec.Placement == nil || spec.Placement.SingleCPU != test.wantCPU) {
			t.Fatal("CPU not applied")
		}
	}
}
