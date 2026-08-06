package operatorchat

import "testing"

func TestRingEvictsOldestOnceFull(t *testing.T) {
	r := newRing(2)
	r.push(Item{Sequence: 1})
	r.push(Item{Sequence: 2})
	r.push(Item{Sequence: 3})

	if r.len() != 2 {
		t.Fatalf("len() = %d, want 2", r.len())
	}
	if r.oldestSequence() != 2 {
		t.Errorf("oldestSequence() = %d, want 2 (sequence 1 evicted)", r.oldestSequence())
	}
	if r.newestSequence() != 3 {
		t.Errorf("newestSequence() = %d, want 3", r.newestSequence())
	}
}

func TestRingAfterReturnsAscendingOrder(t *testing.T) {
	r := newRing(5)
	for seq := uint64(1); seq <= 4; seq++ {
		r.push(Item{Sequence: seq})
	}

	got := r.after(1)
	if len(got) != 3 {
		t.Fatalf("after(1) len = %d, want 3", len(got))
	}
	for i, item := range got {
		want := uint64(i) + 2
		if item.Sequence != want {
			t.Errorf("after(1)[%d].Sequence = %d, want %d", i, item.Sequence, want)
		}
	}
}

func TestRingAfterOnEmptyRing(t *testing.T) {
	r := newRing(3)
	if got := r.after(0); len(got) != 0 {
		t.Errorf("after(0) on empty ring = %v, want empty", got)
	}
	if r.oldestSequence() != 0 || r.newestSequence() != 0 {
		t.Errorf("oldest/newest on empty ring = %d/%d, want 0/0", r.oldestSequence(), r.newestSequence())
	}
}
