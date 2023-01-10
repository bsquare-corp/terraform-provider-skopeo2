//go:build !linux
// +build !linux

package skopeo

func ReexecIfNecessaryForImages(inputImageNames ...string) error {
	return nil
}
