//go:build !linux

package probe

import (
	"context"
	"fmt"
)

// senseImpl is a stub for non-Linux systems.
func senseImpl(ctx context.Context, iface string, channels []uint32, dwellMs uint32) (string, []WlanStation, []WlanChannelStat, uint32, error) {
	return iface, nil, nil, 0, fmt.Errorf("WLAN monitor-mode sensing is only supported on Linux")
}
