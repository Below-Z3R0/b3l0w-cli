package snapshots

import "testing"

func TestParseEntries(t *testing.T) {
	out := `Version           : 1.32.1
Snapshot entries  : 39 (max: auto)
Boot usage        : 30.13% of 3.99 GiB (max 85.0%)
`
	n, err := parseEntries(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 39 {
		t.Fatalf("esperaba 39, obtuve %d", n)
	}
}

func TestParseEntriesMissing(t *testing.T) {
	if _, err := parseEntries("nada útil"); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestParseIDs(t *testing.T) {
	out := `ID  │ Date                │ Description
─────┼─────────────────────┼────────────────────────
 350 │ 2026-10-19 19:53:23 │ seanime-bin
 453 │ 2026-11-03 01:39:10 │
 454 │ 2026-11-03 01:39:27 │ manual
`
	ids := parseIDs(out)
	want := []int{350, 453, 454}
	if len(ids) != len(want) {
		t.Fatalf("esperaba %v, obtuve %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("esperaba %v, obtuve %v", want, ids)
		}
	}
}
