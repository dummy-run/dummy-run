package page

// AxeSource returns the bundled axe-core script. The accessibility sweep injects the same copy
// the a11y tool uses, so both report against one version of the rules.
func AxeSource() string { return axeJS }
