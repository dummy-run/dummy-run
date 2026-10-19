//go:build !unix

package review

// lockDir does nothing where there is no advisory lock. The compare before the rename in
// editOnce still catches a record that changed since it was read.
func lockDir(string) (func(), error) {
	return func() {}, nil
}
