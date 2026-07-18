//go:build !linux

package probe

import (
	"context"
	"errors"
)

func wlanActiveImpl(ctx context.Context, iface string, opts WlanActiveOpts) (*WlanActiveOutcome, error) {
	return nil, errors.New("wlan_active is only supported on Linux")
}
