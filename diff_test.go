package vaultsync

import (
	"strings"
	"testing"
)

// A line inserted at the top must be reported as a single addition, with the
// following lines treated as unchanged context — not as a cascade of edits.
func TestGenerateUnifiedDiffInsertionAtTopDoesNotMislabelFollowingLines(t *testing.T) {
	existing := "alpha\nbravo\ncharlie\n"
	updated := "zulu\nalpha\nbravo\ncharlie\n"

	diff := generateUnifiedDiff(existing, updated, "kv/app")

	if !strings.Contains(diff, "+zulu") {
		t.Fatalf("expected inserted line to be marked as added, got:\n%s", diff)
	}
	for _, unchanged := range []string{"alpha", "bravo", "charlie"} {
		if strings.Contains(diff, "-"+unchanged) {
			t.Errorf("line %q was unchanged but was marked as removed:\n%s", unchanged, diff)
		}
	}
}

// A line deleted mid-file must be reported as a single removal; surrounding
// lines stay as context.
func TestGenerateUnifiedDiffDeletionMidFileDoesNotMislabelFollowingLines(t *testing.T) {
	existing := "alpha\nbravo\ncharlie\ndelta\n"
	updated := "alpha\ncharlie\ndelta\n"

	diff := generateUnifiedDiff(existing, updated, "kv/app")

	if !strings.Contains(diff, "-bravo") {
		t.Fatalf("expected deleted line to be marked as removed, got:\n%s", diff)
	}
	for _, unchanged := range []string{"alpha", "charlie", "delta"} {
		if strings.Contains(diff, "+"+unchanged) {
			t.Errorf("line %q was unchanged but was marked as added:\n%s", unchanged, diff)
		}
		if strings.Contains(diff, "-"+unchanged) {
			t.Errorf("line %q was unchanged but was marked as removed:\n%s", unchanged, diff)
		}
	}
}

func TestGenerateUnifiedDiffIdenticalContentProducesNoDiff(t *testing.T) {
	if diff := generateUnifiedDiff("a\nb\n", "a\nb\n", "kv/app"); diff != "" {
		t.Errorf("expected empty diff for identical content, got:\n%s", diff)
	}
}

// difftastic (installed as `difft`) cannot consume a unified diff on stdin, so
// it must not be advertised as an auto-detected pipe tool.
func TestDetectDiffToolDoesNotAdvertiseDifftastic(t *testing.T) {
	for _, tool := range diffPipeTools {
		if tool == "difftastic" || tool == "difft" {
			t.Errorf("difftastic cannot consume a unified diff on stdin and must not be listed, got %q", tool)
		}
	}
}
