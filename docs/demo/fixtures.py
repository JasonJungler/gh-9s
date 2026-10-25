"""Made-up GitHub data for the demo GIFs: an imaginary repo acme/app. Nothing here is real."""
import datetime as _dt

ME = "mira"
URL = "https://github.com/acme/app"


def ago(**kw):
    return (_dt.datetime.now(_dt.timezone.utc) - _dt.timedelta(**kw)).strftime("%Y-%m-%dT%H:%M:%SZ")


def user(login):
    return {"login": login}


def label(name):
    return {"name": name}


LABELS = ["bug", "enhancement", "docs", "good first issue", "performance"]

WORKFLOWS = [
    {"id": 1, "name": "CI", "state": "active", "path": ".github/workflows/ci.yml"},
    {"id": 2, "name": "Release", "state": "active", "path": ".github/workflows/release.yml"},
    {"id": 3, "name": "Docs", "state": "active", "path": ".github/workflows/docs.yml"},
]


def run(i, title, wf, status, concl, branch, event, **ago_kw):
    return {"databaseId": 9000 + i, "displayTitle": title, "workflowName": wf, "status": status, "conclusion": concl,
            "headBranch": branch, "event": event, "createdAt": ago(**ago_kw), "url": URL + "/actions/runs/%d" % (9000 + i)}


RUNS = [
    run(8, "Add retry to the sync worker", "CI", "in_progress", "", "fix/sync-retry", "pull_request", minutes=2),
    run(7, "Add retry to the sync worker", "CI", "completed", "failure", "fix/sync-retry", "pull_request", minutes=41),
    run(6, "Release v2.4.0", "Release", "completed", "success", "main", "push", hours=5),
    run(5, "Merge pull request #212 from acme/docs-cli", "Docs", "completed", "success", "main", "push", hours=5),
    run(4, "Merge pull request #212 from acme/docs-cli", "CI", "completed", "success", "main", "push", hours=5),
    run(3, "Speed up the search index", "CI", "completed", "success", "perf/index", "pull_request", hours=9),
    run(2, "Cache the token lookup", "CI", "completed", "cancelled", "feat/token-cache", "pull_request", days=1),
    run(1, "Bump dependencies", "CI", "completed", "success", "chore/deps", "pull_request", days=2),
]

ISSUES = [
    (231, "Sync worker drops events after a network timeout", "OPEN", ["bug", "performance"], ["mira"], "noor", dict(hours=3)),
    (229, "Search is slow with more than 10k documents", "OPEN", ["performance"], [], "jun", dict(hours=8)),
    (227, "Add a dark theme to the settings page", "OPEN", ["enhancement"], ["leo"], "ana", dict(days=1)),
    (224, "Document the CLI flags in the README", "OPEN", ["docs", "good first issue"], [], "leo", dict(days=2)),
    (219, "Token refresh loops when the clock is skewed", "OPEN", ["bug"], ["mira", "noor"], "jun", dict(days=3)),
    (214, "Export reports as CSV", "OPEN", ["enhancement"], [], "ana", dict(days=5)),
]

ISSUE_BODIES = {
    231: """The sync worker loses events when the network drops for more than 30 seconds.

**Steps to reproduce**

1. Start the worker with `acme sync --watch`
2. Disconnect the network for a minute
3. Reconnect and check the event count

**Expected:** every event is delivered once.
**Actual:** the events sent during the outage are gone.

```go
func (w *Worker) send(ev Event) error {
    ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
    defer cancel()
    return w.client.Post(ctx, ev) // the error is not retried
}
```

> Seen on 2.3.1 and 2.4.0.
""",
}


def issue_row(n, t, st, labels, assg, author, when):
    return {"number": n, "title": t, "author": user(author), "state": st, "labels": [label(x) for x in labels],
            "assignees": [user(a) for a in assg], "updatedAt": ago(**when), "url": URL + "/issues/%d" % n}


ISSUE_LIST = [issue_row(*i) for i in ISSUES]


def issue(n):
    n = int(n)
    row = next((i for i in ISSUES if i[0] == n), (n, "Pull request #%d" % n, "OPEN", [], [], "leo", dict(days=1)))
    d = issue_row(*row)
    d["body"] = ISSUE_BODIES.get(n, "Details to follow.")
    d["comments"] = [
        {"author": user("noor"), "body": "I can reproduce this on 2.4.0. The worker logs `context deadline exceeded` and moves on.", "createdAt": ago(hours=2)},
        {"author": user("mira"), "body": "Thanks. I'll add a retry with backoff, see #233.", "createdAt": ago(hours=1)},
    ]
    return d


PRS = [
    (233, "Add retry with backoff to the sync worker", "fix/sync-retry", False, "REVIEW_REQUIRED", "mira", dict(minutes=50)),
    (232, "Cache the token lookup", "feat/token-cache", False, "APPROVED", "jun", dict(hours=6)),
    (228, "Speed up the search index", "perf/index", False, "CHANGES_REQUESTED", "noor", dict(hours=10)),
    (226, "WIP: dark theme", "feat/dark-theme", True, "", "ana", dict(days=1)),
    (222, "Bump dependencies", "chore/deps", False, "APPROVED", "leo", dict(days=2)),
]

PR_LIST = [
    {"number": n, "title": t, "author": user(a), "headRefName": br, "isDraft": dr, "reviewDecision": rd,
     "updatedAt": ago(**when), "url": URL + "/pull/%d" % n}
    for (n, t, br, dr, rd, a, when) in PRS
]

OK = [{"__typename": "CheckRun", "name": "build", "status": "COMPLETED", "conclusion": "SUCCESS"}]
BAD = [{"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "FAILURE"}]
WAIT = [{"__typename": "CheckRun", "name": "build", "status": "IN_PROGRESS", "conclusion": ""}]
PR_CHECKS = [{"number": 233, "statusCheckRollup": OK * 3}, {"number": 232, "statusCheckRollup": OK * 3},
             {"number": 228, "statusCheckRollup": OK + BAD}, {"number": 226, "statusCheckRollup": WAIT},
             {"number": 222, "statusCheckRollup": OK * 3}]

DIFF = """diff --git a/internal/sync/worker.go b/internal/sync/worker.go
index 4c1e2a7..9b83f10 100644
--- a/internal/sync/worker.go
+++ b/internal/sync/worker.go
@@ -12,14 +12,31 @@ import (
 	"context"
 	"time"
+	"math/rand"
 )
 
+const maxAttempts = 5
+
 func (w *Worker) send(ev Event) error {
-	ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
-	defer cancel()
-	return w.client.Post(ctx, ev)
+	var err error
+	for attempt := 0; attempt < maxAttempts; attempt++ {
+		ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
+		err = w.client.Post(ctx, ev)
+		cancel()
+		if err == nil {
+			return nil
+		}
+		time.Sleep(backoff(attempt))
+	}
+	return fmt.Errorf("giving up after %d attempts: %w", maxAttempts, err)
 }
+
+// backoff waits 1s, 2s, 4s... plus up to a second of jitter.
+func backoff(attempt int) time.Duration {
+	base := time.Second << attempt
+	return base + time.Duration(rand.Intn(1000))*time.Millisecond
+}
diff --git a/internal/sync/worker_test.go b/internal/sync/worker_test.go
index 0000000..51d2c3e 100644
--- a/internal/sync/worker_test.go
+++ b/internal/sync/worker_test.go
@@ -1,3 +1,18 @@
 package sync
 
+func TestSendRetriesUntilTheServerAnswers(t *testing.T) {
+	srv := flaky(t, 3) // fails three times, then answers
+	w := newWorker(srv.URL)
+	if err := w.send(Event{ID: "e1"}); err != nil {
+		t.Fatal(err)
+	}
+	if srv.calls != 4 {
+		t.Fatalf("calls = %d, want 4", srv.calls)
+	}
+}
"""


def pr(n):
    n = int(n)
    row = next(p for p in PRS if p[0] == n)
    _, t, br, dr, rd, a, when = row
    return {
        "number": n, "title": t, "body": "Retries a failed send up to five times with exponential backoff and jitter.\n\nFixes #231.\n\n- [x] test for the flaky server\n- [x] docs",
        "author": user(a), "state": "OPEN", "isDraft": dr, "baseRefName": "main", "headRefName": br, "headRefOid": "9b83f10c2a",
        "url": URL + "/pull/%d" % n, "reviewDecision": rd, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
        "additions": 31, "deletions": 4, "changedFiles": 2, "labels": [label("bug")],
        "comments": [{"author": user("noor"), "body": "Looks good. Could the limit be configurable?", "createdAt": ago(minutes=30)}],
        "reviews": [], "reviewRequests": [{"login": "noor"}, {"login": "jun"}], "latestReviews": [],
        "assignees": [user("mira")], "projectItems": [], "milestone": {"title": "v2.5"},
        "closingIssuesReferences": [{"number": 231, "title": "Sync worker drops events after a network timeout"}],
        "statusCheckRollup": [
            {"__typename": "CheckRun", "name": "build", "status": "COMPLETED", "conclusion": "SUCCESS"},
            {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "SUCCESS"},
            {"__typename": "CheckRun", "name": "lint", "status": "COMPLETED", "conclusion": "SUCCESS"},
        ],
    }


def review_threads():
    return {"data": {"repository": {"pullRequest": {"reviewThreads": {"nodes": []}}}}}


# ---- the Project board -------------------------------------------------------------------------------------------

OPTS = [("opt_todo", "Todo"), ("opt_prog", "In Progress"), ("opt_review", "In Review"), ("opt_done", "Done")]


def status_name(opt):
    return dict(OPTS).get(opt, "Todo")


CARDS = [
    # id, kind, number, title, status, assignees, labels, state
    ("PVTI_1", "Issue", 231, "Sync worker drops events after a network timeout", "In Progress", ["mira"], ["bug"], "OPEN"),
    ("PVTI_2", "PullRequest", 233, "Add retry with backoff to the sync worker", "In Review", ["mira"], ["bug"], "OPEN"),
    ("PVTI_3", "Issue", 229, "Search is slow with more than 10k documents", "Todo", [], ["performance"], "OPEN"),
    ("PVTI_4", "Issue", 227, "Add a dark theme to the settings page", "Todo", ["leo"], ["enhancement"], "OPEN"),
    ("PVTI_5", "Issue", 224, "Document the CLI flags in the README", "Todo", [], ["docs"], "OPEN"),
    ("PVTI_6", "PullRequest", 232, "Cache the token lookup", "In Review", ["jun"], ["enhancement"], "OPEN"),
    ("PVTI_7", "Issue", 219, "Token refresh loops when the clock is skewed", "In Progress", ["mira", "noor"], ["bug"], "OPEN"),
    ("PVTI_8", "PullRequest", 222, "Bump dependencies", "Done", ["leo"], [], "MERGED"),
    ("PVTI_9", "PullRequest", 212, "Document the CLI commands", "Done", ["ana"], ["docs"], "MERGED"),
    ("PVTI_10", "Issue", 214, "Export reports as CSV", "Todo", [], ["enhancement"], "OPEN"),
]


def project_list():
    n = lambda i, t: {"id": "PVT_%d" % i, "number": i, "title": t, "closed": False, "url": "https://github.com/orgs/acme/projects/%d" % i}
    return {"data": {"viewer": {"login": ME, "projectsV2": {"nodes": []},
                                "organizations": {"nodes": [{"login": "acme", "projectsV2": {"nodes": [n(1, "Sprint board"), n(2, "Roadmap")]}}]}}}}


def project_page(s):
    nodes = []
    for (iid, kind, num, title, status, assg, labels, st) in CARDS:
        if iid in s["gone"]:
            continue
        status = s["status"].get(iid, status)
        content = {"__typename": kind, "number": num, "title": title, "state": st, "url": URL + ("/pull/%d" if kind == "PullRequest" else "/issues/%d") % num,
                   "updatedAt": ago(hours=num % 20 + 1), "repository": {"nameWithOwner": "acme/app"},
                   "assignees": {"nodes": [user(a) for a in assg]}, "labels": {"nodes": [label(x) for x in labels]}}
        if kind == "PullRequest":
            content.update({"isDraft": False, "merged": st == "MERGED"})
        nodes.append({"id": iid, "fieldValues": {"nodes": []}, "fieldValueByName": {"name": status}, "content": content})
    for d in s["drafts"]:
        nodes.append({"id": d["id"], "fieldValues": {"nodes": []}, "fieldValueByName": None,
                      "content": {"__typename": "DraftIssue", "id": "DI_" + d["id"], "title": d["title"], "body": ""}})
    return {"data": {"repositoryOwner": {"projectV2": {
        "id": "PVT_1", "title": "Sprint board", "closed": False, "url": "https://github.com/orgs/acme/projects/1",
        "views": {"pageInfo": {"hasNextPage": False}, "nodes": [
            {"name": "Board", "layout": "BOARD_LAYOUT", "filter": "", "fields": {"nodes": []}, "sortByFields": {"nodes": []}},
            {"name": "My cards", "layout": "TABLE_LAYOUT", "filter": "assignee:@me", "fields": {"nodes": []}, "sortByFields": {"nodes": []}}]},
        "field": {"id": "FLD_status", "options": [{"id": i, "name": n} for i, n in OPTS]},
        "fields": {"nodes": []},
        "items": {"totalCount": len(nodes), "pageInfo": {"hasNextPage": False, "endCursor": ""}, "nodes": nodes}}}}}


# ---- one run's jobs, workflow file and log ------------------------------------------------------------------------

CI_YML = """name: CI
on: [push, pull_request]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: golangci-lint run
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go build ./...
  test:
    needs: [build]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
  package:
    needs: [test, lint]
    runs-on: ubuntu-latest
    steps:
      - run: make package
"""


def _job(i, name, status, concl, start, end, steps):
    t = lambda s: (_dt.datetime.now(_dt.timezone.utc) - _dt.timedelta(minutes=45) + _dt.timedelta(seconds=s)).strftime("%Y-%m-%dT%H:%M:%SZ")
    return {"databaseId": 7000 + i, "name": name, "status": status, "conclusion": concl, "startedAt": t(start), "completedAt": t(end),
            "url": URL + "/actions/runs/9007/job/%d" % (7000 + i),
            "steps": [{"name": n, "status": "completed", "conclusion": c} for n, c in steps]}


def run_view(run_id):
    ok = "success"
    return {"status": "completed", "conclusion": "failure", "headSha": "9b83f10c2a", "workflowDatabaseId": 1, "jobs": [
        _job(1, "lint", "completed", ok, 0, 41, [("Set up job", ok), ("Run golangci-lint", ok)]),
        _job(2, "build", "completed", ok, 0, 58, [("Set up job", ok), ("Run go build", ok)]),
        _job(3, "test", "completed", "failure", 60, 131, [("Set up job", ok), ("Run go test", "failure")]),
        _job(4, "package", "completed", "skipped", 131, 131, [("Set up job", "skipped")]),
    ]}


def job_log():
    ts = "2026-10-25T09:%02d:%02d.000Z"
    lines = [
        "=== RUN   TestSendRetriesUntilTheServerAnswers",
        "--- PASS: TestSendRetriesUntilTheServerAnswers (3.02s)",
        "=== RUN   TestSendGivesUpAfterFiveAttempts",
        "    worker_test.go:42: calls = 4, want 5",
        "--- FAIL: TestSendGivesUpAfterFiveAttempts (4.10s)",
        "=== RUN   TestBackoffGrows",
        "--- PASS: TestBackoffGrows (0.00s)",
        "FAIL",
        "FAIL\tacme/app/internal/sync\t7.151s",
        "Error: Process completed with exit code 1.",
    ]
    return "".join("test\tRun go test\t" + ts % (14, i * 3) + " " + l + "\n" for i, l in enumerate(lines))


# ---- Discussions -------------------------------------------------------------------------------------------------

CATS = [("DIC_qa", "Q&A", True), ("DIC_ideas", "Ideas", False), ("DIC_general", "General", False)]
DISCUSSIONS = [
    (42, "How do I retry a failed sync?", "Q&A", "ann", True, False, False, 3, dict(hours=2), "I run `acme sync` and it stops on the first timeout.\n\nIs there a way to retry?"),
    (41, "Dark theme for the settings page", "Ideas", "leo", False, False, False, 5, dict(hours=9), "It would be easier on the eyes at night."),
    (39, "Release notes for 2.4", "General", "mira", False, True, False, 0, dict(days=2), "Draft notes are in the wiki."),
    (37, "Sync worker memory use", "Q&A", "noor", False, False, False, 2, dict(days=3), "Memory grows by 5 MB per hour. Anyone seen this?"),
]


def discussion_list(cat_id):
    nodes = []
    for (n, t, c, a, ans, closed, locked, cm, when, body) in DISCUSSIONS:
        if cat_id and dict((i, nm) for i, nm, _ in CATS).get(cat_id) != c:
            continue
        nodes.append({"id": "D_%d" % n, "number": n, "title": t, "body": body, "url": URL + "/discussions/%d" % n, "updatedAt": ago(**when),
                      "closed": closed, "locked": locked, "isAnswered": ans, "author": user(a), "category": {"name": c}, "comments": {"totalCount": cm}})
    return {"data": {"repository": {"hasDiscussionsEnabled": True,
                                    "discussionCategories": {"nodes": [{"id": i, "name": n, "isAnswerable": q} for i, n, q in CATS]},
                                    "discussions": {"totalCount": len(nodes), "nodes": nodes}}}}


def discussion():
    d = DISCUSSIONS[0]
    return {"data": {"repository": {"hasDiscussionsEnabled": True, "discussion": {
        "id": "D_42", "number": 42, "title": d[1], "body": d[9], "url": URL + "/discussions/42", "closed": False, "locked": False, "isAnswered": True,
        "createdAt": ago(hours=5), "author": user("ann"), "category": {"name": "Q&A"},
        "comments": {"totalCount": 2, "nodes": [
            {"id": "K1", "body": "Wrap the call in a loop with a backoff, see #233.", "createdAt": ago(hours=4), "isAnswer": True, "author": user("mira"),
             "replies": {"totalCount": 2, "nodes": [
                 {"id": "R1", "body": "That worked, thank you!", "createdAt": ago(hours=3), "author": user("ann")},
                 {"id": "R2", "body": "Glad it did. The fix ships in 2.5.", "createdAt": ago(hours=2), "author": user("mira")}]}},
            {"id": "K2", "body": "You can also set `SYNC_RETRIES=5`.", "createdAt": ago(hours=1), "isAnswer": False, "author": user("noor"),
             "replies": {"totalCount": 0, "nodes": []}}]}}}}}
