package state

import (
	"testing"
	"time"
)

func TestRoundTripAndPrune(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Unix()
	st := &State{}
	st.MarkQueued("new", now)
	st.MarkQueued("old", now-61*86400)
	st.LastUpdateCheck = 42
	if err := Save(dir, st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAt(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Has("new") || got.Has("old") || got.LastUpdateCheck != 42 {
		t.Fatalf("%+v", got)
	}
	got2, err := Load(dir)
	if err != nil || !got2.Has("new") {
		t.Fatalf("%+v %v", got2, err)
	}
}

func TestLoadMissing(t *testing.T) {
	st, err := Load(t.TempDir())
	if err != nil || st.Sent == nil || st.Has("x") {
		t.Fatalf("%+v %v", st, err)
	}
	st.MarkQueued("x", 1)
}
