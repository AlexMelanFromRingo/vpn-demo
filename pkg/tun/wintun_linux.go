// +build linux

package tun

// EnsureWintun is a no-op on Linux
func EnsureWintun() error {
	return nil
}
