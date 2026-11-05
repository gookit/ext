//go:build !windows && !linux

package servicemgr

import "context"

// Register is not supported on this platform.
func Register(ctx context.Context, s Spec) error { return ErrUnsupported }

// Unregister is not supported on this platform.
func Unregister(ctx context.Context, name, owner string) error { return ErrUnsupported }

// Start is not supported on this platform.
func Start(ctx context.Context, name, owner string) error { return ErrUnsupported }

// Query is not supported on this platform.
func Query(ctx context.Context, name string) (Status, error) { return Status{}, ErrUnsupported }
