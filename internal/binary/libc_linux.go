//go:build linux

package binary

type defaultLibcDetector struct{}

func (d *defaultLibcDetector) Detect() (LibcType, error) {
	return detectLibcFromRoot("/")
}

// DefaultLibcDetector returns the platform libc detector for Linux.
func DefaultLibcDetector() LibcDetector {
	return &defaultLibcDetector{}
}
