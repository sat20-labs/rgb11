package rejectlist

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sat20-labs/rgb11/operations"
)

func testOpout(fill string) operations.Opout {
	opout, err := operations.ParseOpout(strings.Repeat(fill, 64) + "/4000/0")
	if err != nil {
		panic(err)
	}
	return opout
}

func TestClientRestrictsHTTPToRegtestLoopback(t *testing.T) {
	opout := testOpout("b")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(opout.String() + "\n"))
	}))
	defer server.Close()
	if _, err := (Client{}).Fetch(server.URL); !errors.Is(err, ErrURLPolicy) {
		t.Fatalf("production HTTP error=%v", err)
	}
	list, err := (Client{AllowLoopbackHTTP: true}).Fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := list.Reject[opout]; !ok {
		t.Fatal("loopback response was not parsed")
	}
}

func TestClientRejectsCrossHostRedirectAndOversizedResponse(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ignored"))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := (Client{AllowLoopbackHTTP: true}).Fetch(redirect.URL); !errors.Is(err, ErrService) {
		t.Fatalf("redirect error=%v", err)
	}
	overflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, int(MaxResponseBytes)+1))
	}))
	defer overflow.Close()
	if _, err := (Client{AllowLoopbackHTTP: true}).Fetch(overflow.URL); !errors.Is(err, ErrResponseLimit) {
		t.Fatalf("overflow error=%v", err)
	}
}

func TestOfficialRejectListAncestryScenarios(t *testing.T) {
	root := testOpout("1")
	child := testOpout("2")
	grandchild := testOpout("3")
	sibling := testOpout("4")
	dag := map[operations.Opout][]operations.Opout{
		root: nil, child: {root}, grandchild: {child}, sibling: {root},
	}
	tests := []struct {
		name    string
		raw     string
		checked operations.Opout
		want    bool
	}{
		{"current opout rejected", child.String(), child, true},
		{"ancestor rejected", root.String(), grandchild, true},
		{"later allow overrides reject", child.String() + "\n!" + child.String(), child, false},
		{"allowed descendant shields older reject", root.String() + "\n!" + child.String(), grandchild, false},
		{"rejected sibling is irrelevant", sibling.String(), grandchild, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := Parse([]byte(test.raw)).RejectedAncestor(test.checked, dag)
			if got != test.want {
				t.Fatalf("rejected=%v, want %v", got, test.want)
			}
		})
	}
}

func TestParseIgnoresInvalidLinesAndLastDecisionWins(t *testing.T) {
	opout := testOpout("a")
	list := Parse([]byte("# comment\nnot-an-opout\n!" + opout.String() + "\n" + opout.String() + "\n"))
	if _, ok := list.Reject[opout]; !ok {
		t.Fatal("expected final reject decision")
	}
	if _, ok := list.Allow[opout]; ok {
		t.Fatal("opout must not remain in both sets")
	}
}
