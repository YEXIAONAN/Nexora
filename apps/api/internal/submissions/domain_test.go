package submissions

import (
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestEvidenceNormalizationRejectsUnsafeOrEmptyContent(t *testing.T) {
	for _, in := range []SubmitInput{
		{ClientRequestID: uuid.New()},
		{Summary: "Evidence"},
		{ClientRequestID: uuid.New(), References: []Reference{{Kind: "link", URL: "javascript:alert(1)"}}},
		{ClientRequestID: uuid.New(), References: []Reference{{Kind: "link", URL: "data:text/html,hello"}}},
		{ClientRequestID: uuid.New(), References: []Reference{{Kind: "link", URL: "https://user:pass@example.com/evidence"}}},
		{ClientRequestID: uuid.New(), References: []Reference{{Kind: "link", URL: "https://example.com/\nevidence"}}},
		{ClientRequestID: uuid.New(), Summary: strings.Repeat("界", 8001)},
		{ClientRequestID: uuid.New(), Summary: "Evidence", AttachmentIDs: []uuid.UUID{uuid.Nil}},
	} {
		if _, e := normalize(&in); e == nil {
			t.Fatalf("accepted unsafe/empty content: %+v", in)
		}
	}
	in := SubmitInput{ClientRequestID: uuid.New(), References: []Reference{{Kind: "git_commit", URL: "https://example.com/repo/commit/abc", Label: "Commit reference"}}}
	a, e := normalize(&in)
	if e != nil || len(a) != 64 {
		t.Fatal(a, e)
	}
	in.ExpectedVersion = 100
	in.ClientRequestID = uuid.New()
	b, e := normalize(&in)
	if e != nil || a != b {
		t.Fatal("retry digest includes transport concurrency fields")
	}
}
