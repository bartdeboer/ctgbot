package messaging

import "testing"

func TestThreadMatchesQueryMatchesShortID(t *testing.T) {
	thread := ThreadSummary{
		ShortID: "00VHFc",
	}
	if !threadMatchesQuery(thread, "00vhfc") {
		t.Fatal("threadMatchesQuery() did not match ShortID")
	}
}
