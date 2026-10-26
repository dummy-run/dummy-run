package a11y

// mergeRepeats folds the results of each repeat of the sweep into one result for each page and setting. Every
// element records in how many of the repeats it failed, and every result in how many its page could not be
// tested. It counts and judges nothing: which counts make a finding stable is for you to decide.
func mergeRepeats(runs [][]PageResult) []PageResult {
	if len(runs) == 0 {
		return nil
	}
	n := len(runs)
	out := make([]PageResult, 0, len(runs[0]))
	at := map[string]int{}
	for _, run := range runs {
		for _, p := range run {
			key := p.URL + "|" + p.Setting()
			i, seen := at[key]
			if !seen {
				p.Violations = countNodes(nil, p.Violations)
				p.NeedsReview = countNodes(nil, p.NeedsReview)
				p.Advice = countNodes(nil, p.Advice)
				p.Candidates = countNodes(nil, p.Candidates)
				p.Repeats = n
				if p.Error != "" {
					p.ErrorRuns = 1
				}
				at[key] = len(out)
				out = append(out, p)
				continue
			}
			base := &out[i]
			base.Violations = countNodes(base.Violations, p.Violations)
			base.NeedsReview = countNodes(base.NeedsReview, p.NeedsReview)
			base.Advice = countNodes(base.Advice, p.Advice)
			base.Candidates = countNodes(base.Candidates, p.Candidates)
			if p.Error != "" {
				base.ErrorRuns++
				if base.Error == "" {
					base.Error = p.Error
				}
			}
		}
	}
	return out
}

// countNodes adds one repeat's findings to the ones so far: an element already there counts one more run, and a
// new one joins its rule, or a new copy of the rule, with one run.
func countNodes(into, more []Finding) []Finding {
	if into == nil && more != nil {
		into = []Finding{}
	}
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
		for _, nd := range f.Nodes {
			found := false
			for j := range into[fi].Nodes {
				if into[fi].Nodes[j].Target == nd.Target {
					into[fi].Nodes[j].Runs++
					found = true
					break
				}
			}
			if !found {
				nd.Runs = 1
				into[fi].Nodes = append(into[fi].Nodes, nd)
			}
		}
	}
	return into
}
