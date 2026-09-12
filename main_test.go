package main

import (
	"fmt"
	"testing"

	"github.com/shurcooL/githubv4"
)

const pnpmPrefix = "[nvchecker] sys-apps/pnpm can be bump to "

func closedPnpm(n int) Issue {
	return Issue{Number: githubv4.Int(n), Title: githubv4.String(fmt.Sprintf("%s11.%d.0", pnpmPrefix, n)), State: githubv4.IssueStateClosed}
}

// Two pages: the first holds 20 closed pnpm issues and the open pnpm-bin
// issue; the open pnpm issue only appears on the second page.
func pagedPnpm() func(after *githubv4.String) issuePage {
	cursor := githubv4.String("page2")
	return func(after *githubv4.String) issuePage {
		if after == nil {
			page := issuePage{next: &cursor}
			page.issues = append(page.issues, Issue{Number: 1, Title: "[nvchecker] sys-apps/pnpm-bin can be bump to 12.4.1", State: githubv4.IssueStateOpen})
			for n := 2; n <= 21; n++ {
				page.issues = append(page.issues, closedPnpm(n))
			}
			return page
		}
		return issuePage{issues: []Issue{
			{Number: 99, Title: pnpmPrefix + "12.4.1", State: githubv4.IssueStateOpen},
		}}
	}
}

func TestOpenIssueSkipsBinSiblingAndPages(t *testing.T) {
	got := findIssue(pagedPnpm(), openIssueFor(pnpmPrefix))
	if got.Number != 99 {
		t.Fatalf("picked issue %d, want 99 (exact prefix, second page)", got.Number)
	}
}

func TestOpenIssueIgnoresClosed(t *testing.T) {
	fetch := func(after *githubv4.String) issuePage {
		return issuePage{issues: []Issue{closedPnpm(5)}}
	}
	if got := findIssue(fetch, openIssueFor(pnpmPrefix)); got != (Issue{}) {
		t.Fatalf("picked closed issue %d, want none", got.Number)
	}
}

func TestSameIssueNeedsTitleAndBody(t *testing.T) {
	title := pnpmPrefix + "12.4.1"
	match := sameIssue(title, "body")
	if !match(Issue{Title: githubv4.String(title), Body: "body"}) {
		t.Fatal("identical title and body did not match")
	}
	if match(Issue{Title: githubv4.String(title), Body: "other"}) {
		t.Fatal("different body matched")
	}
	if match(Issue{Title: "[nvchecker] sys-apps/pnpm-bin can be bump to 12.4.1", Body: "body"}) {
		t.Fatal("-bin sibling matched")
	}
}

func TestFindIssueStopsWithoutNextPage(t *testing.T) {
	calls := 0
	fetch := func(after *githubv4.String) issuePage {
		calls++
		return issuePage{}
	}
	findIssue(fetch, openIssueFor(pnpmPrefix))
	if calls != 1 {
		t.Fatalf("fetched %d pages, want 1", calls)
	}
}
