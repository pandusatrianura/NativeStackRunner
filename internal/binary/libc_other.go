//go:build !linux

package binary

type defaultLibcDetector struct{}

func (d *defaultLibcDetector) Detect() (LibcType, error) {
	return LibcUnknown, nil
}

// DefaultLibcDetector returns a stub libc detector on non-Linux platforms.
func DefaultLibcDetector() LibcDetector {
	return &defaultLibcDetector{}
}
