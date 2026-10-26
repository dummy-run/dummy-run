package a11y

import "strconv"

// mergeRepeats folds the results of each repeat of the sweep into one result for each page and setting. Every
// element records in how many of the repeats it failed, and every result in how many its page could not be
// tested. It counts and judges nothing: which counts make a finding stable is for you to decide.
//
// A URL that appears twice in one repeat, such as from the sitemap and the command line, keeps two results, as it
// would without --repeat, and each counts once per repeat. The result kept for a page is the first one whose page
// could be tested, so the checks it records are the ones that ran, with the first error kept beside it.
func mergeRepeats(runs [][]PageResult) []PageResult {
	if len(runs) == 0 {
		return nil
	}
	n := len(runs)
	var order []string
	groups := map[string][]PageResult{}
	for _, run := range runs {
		seen := map[string]int{}
		for _, p := range run {
			base := p.URL + "|" + p.Setting()
			key := base + "|" + strconv.Itoa(seen[base])
			seen[base]++
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], p)
		}
	}
	out := make([]PageResult, 0, len(order))
	for _, key := range order {
		out = append(out, mergeGroup(groups[key], n))
	}
	return out
}

// mergeGroup merges one page and setting's results from every repeat.
func mergeGroup(ps []PageResult, repeats int) PageResult {
	pick := 0
	for i, p := range ps {
		if p.Error == "" {
			pick = i
			break
		}
	}
	m := ps[pick]
	// Only the first repeat takes the --visual picture, so its result has it whichever result is kept.
	m.Visual, m.VisualError = ps[0].Visual, ps[0].VisualError
	// Only the first repeat takes the ARIA snapshot, so its result has it whichever result is kept.
	m.ARIASnapshot, m.ARIASnapshotOmitted, m.ARIAChanges, m.ARIASnapshotError = ps[0].ARIASnapshot, ps[0].ARIASnapshotOmitted, ps[0].ARIAChanges, ps[0].ARIASnapshotError
	m.Violations, m.NeedsReview, m.Advice, m.Candidates = []Finding{}, []Finding{}, nil, nil
	errorRuns := 0
	m.Repeats, m.ErrorRuns, m.Error = repeats, &errorRuns, ""
	// The kept result's elements come first, then any that only the others found.
	for _, i := range append([]int{pick}, others(len(ps), pick)...) {
		p := ps[i]
		m.Violations = countNodes(m.Violations, p.Violations)
		m.NeedsReview = countNodes(m.NeedsReview, p.NeedsReview)
		m.Advice = countNodes(m.Advice, p.Advice)
		m.Candidates = countNodes(m.Candidates, p.Candidates)
	}
	for _, p := range ps {
		if p.Error != "" {
			errorRuns++
			if m.Error == "" {
				m.Error = p.Error
			}
		}
	}
	return m
}

func others(n, skip int) []int {
	out := make([]int, 0, n)
	for i := range n {
		if i != skip {
			out = append(out, i)
		}
	}
	return out
}

// countNodes adds one repeat's findings to the ones so far: an element already there counts one more run, and a
// new one joins its rule, or a new copy of the rule, with one run. A check can report one element more than once in
// a run, such as clipped text in two places inside one control, so the n-th report of an element in this repeat
// is matched with its n-th report so far, and no element counts twice in one repeat.
func countNodes(into, more []Finding) []Finding {
	seen := map[string]int{}
	for _, f := range more {
		fi := -1
		for i := range into {
			if into[i].Rule == f.Rule {
				fi = i
				break
			}
		}
		if fi < 0 {
			g := f
			g.Nodes = nil
			into = append(into, g)
			fi = len(into) - 1
		}
		for _, node := range f.Nodes {
			nth := seen[f.Rule+"\x00"+node.Target]
			seen[f.Rule+"\x00"+node.Target]++
			found := false
			for j := range into[fi].Nodes {
				if into[fi].Nodes[j].Target != node.Target {
					continue
				}
				if nth == 0 {
					into[fi].Nodes[j].Runs++
					found = true
					break
				}
				nth--
			}
			if !found {
				node.Runs = 1
				into[fi].Nodes = append(into[fi].Nodes, node)
			}
		}
	}
	return into
}
