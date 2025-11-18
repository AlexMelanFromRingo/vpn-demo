package tun

import "os"

// device is an internal interface for platform-specific TUN implementations
type device interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	File() *os.File // For Linux only
}

// tunDevice is a simple file-based TUN device (Linux)
type tunDevice struct {
	file *os.File
}

func (t *tunDevice) Read(buf []byte) (int, error) {
	return t.file.Read(buf)
}

func (t *tunDevice) Write(buf []byte) (int, error) {
	return t.file.Write(buf)
}

func (t *tunDevice) Close() error {
	return t.file.Close()
}

func (t *tunDevice) File() *os.File {
	return t.file
}
