// Package incumbent is the frozen production comparator captured from commit
// 377ac019 (the post-PR58 engine). The deliberate source copy keeps later
// search and evaluator work from changing the baseline under measurement.
//
// At the freeze commit, reproduce the normalized copy comparison from the
// repository root with:
//
//	diff -u <(sed -e 's/^package search$/package incumbent/' -e 's#^// Package search chooses Virus actions using deterministic anytime search.$#// Package incumbent freezes the post-PR58 authoritative search and evaluator.#' backend/search/search.go) backend/search/incumbent/search.go
//	diff -u <(sed 's/^package search$/package incumbent/' backend/search/evaluate.go) backend/search/incumbent/evaluate.go
//
// A later current-engine change is expected to make this diff non-empty; the
// checked-in golden tests, rather than parity with mutable code, guard this
// package after that point.
package incumbent
