package agentmemory

// FoldPlan is the decision [Fold] reaches for one curation pass: the curated
// contents to add as new pending proposals, and the ids of stale pending
// proposals to remove. It carries no persistence detail — the store mints ids
// for the inserts and applies the deletes.
type FoldPlan struct {
	InsertContents []string
	PruneIDs       []string
}

// Fold reconciles a curator's output into a project's auto-origin item set,
// enforcing the review invariants:
//
//   - A curated fact not yet represented (in any status) becomes a new PENDING
//     proposal.
//   - A pending proposal the curator no longer produces is pruned.
//   - Approved (active) items are sticky: what the user accepted is never
//     auto-removed, even if the curator drops it as obsolete.
//   - Rejected items are tombstones: their digest blocks the same fact from
//     being re-proposed, and they are never pruned.
//
// existing is the project's auto-origin lifecycle set (id + content + status
// suffice). The caller excludes pinned visible items and user-authored items;
// rejected tombstones still participate even if they were pinned before
// rejection. The result is deterministic in the order of contents. Invalid or
// overlarge curator output is rejected before persistence can partially apply it.
func Fold(existing []Item, contents []string) (FoldPlan, error) {
	contents, err := normalizeFactList(contents, MaxCurationProposals, "curation result")
	if err != nil {
		return FoldPlan{}, err
	}
	present := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		present[Digest(item.Content)] = struct{}{}
	}

	var plan FoldPlan
	desired := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		digest := Digest(content)
		if _, dup := desired[digest]; dup {
			continue
		}
		desired[digest] = struct{}{}
		if _, seen := present[digest]; !seen {
			plan.InsertContents = append(plan.InsertContents, content)
		}
	}

	for _, item := range existing {
		if item.Status != StatusPending {
			continue // active is sticky, rejected is a tombstone
		}
		if _, keep := desired[Digest(item.Content)]; !keep {
			plan.PruneIDs = append(plan.PruneIDs, item.ID)
		}
	}
	return plan, nil
}
