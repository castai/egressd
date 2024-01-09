// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package conntrack

import (
	"fmt"

	"github.com/cilium/cilium/pkg/datapath/linux/probes"
	"golang.org/x/sys/unix"
)

var hertz uint16

func init() {
	var err error
	hertz, err = getKernelHZ()
	if err != nil {
		hertz = 1
	}
}

type ClockSource string

const (
	ClockSourceKtime   ClockSource = "ktime"
	ClockSourceJiffies ClockSource = "jiffies"
)

// Make linter happy. It's used for linux build target.
var _ = kernelTimeDiffSecondsFunc

// kernelTimeDiffSecondsFunc returns time diff function based on clock source.
func kernelTimeDiffSecondsFunc(clockSource ClockSource) (func(t int64) int64, error) {
	switch clockSource {
	case ClockSourceKtime:
		now, err := getMtime()
		if err != nil {
			return nil, err
		}
		now = now / 1000000000
		return func(t int64) int64 {
			return t - int64(now)
		}, nil
	case ClockSourceJiffies:
		now, err := probes.Jiffies()
		if err != nil {
			return nil, err
		}
		return func(t int64) int64 {
			diff := t - int64(now)
			diff = diff << 8
			diff = diff / int64(hertz)
			return diff
		}, nil
	default:
		return nil, fmt.Errorf("unknown clock source %q", clockSource)
	}
}

// getMtime returns monotonic time that can be used to compare
// values with ktime_get_ns() BPF helper, e.g. needed to check
// the timeout in sec for BPF entries. We return the raw nsec,
// although that is not quite usable for comparison. Go has
// runtime.nanotime() but doesn't expose it as API.
func getMtime() (uint64, error) {
	var ts unix.Timespec

	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	if err != nil {
		return 0, fmt.Errorf("Unable get time: %w", err)
	}

	return uint64(unix.TimespecToNsec(ts)), nil
}

// const (
// 	timerInfoFilepath = "/proc/timer_list"
// )

// getJtime returns a close-enough approximation of kernel jiffies
// that can be used to compare against jiffies BPF helper. We parse
// it from /proc/timer_list. GetJtime() should be invoked only at
// mid-low frequencies.
// func getJtime() (uint64, error) {
// 	jiffies := uint64(0)
// 	scaler := uint64(8)
// 	timers, err := os.Open(timerInfoFilepath)
// 	if err != nil {
// 		return 0, err
// 	}
// 	defer timers.Close()
// 	scanner := bufio.NewScanner(timers)
// 	for scanner.Scan() {
// 		tmp := uint64(0)
// 		n, _ := fmt.Sscanf(scanner.Text(), "jiffies: %d\n", &tmp)
// 		if n == 1 {
// 			jiffies = tmp
// 			break
// 		}
// 	}
// 	return jiffies >> scaler, scanner.Err()
// }
