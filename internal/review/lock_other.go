//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package review

// lockDir does nothing on a system with no folder or file lock in the standard library, such as AIX
// and Solaris. The compare before the rename in editOnce still catches a record that changed since
// it was read, but two reviewers can both pass it before either renames.
func lockDir(string) (func(), error) {
	return func() {}, nil
}
