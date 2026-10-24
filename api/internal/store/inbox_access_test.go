package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var (
	inboxMembersRef = regexp.MustCompile(`(?i)\binbox_members\b`)
	sqlComment      = regexp.MustCompile(`--[^\n]*`)
	viewOrFunction  = regexp.MustCompile(`(?i)\bcreate\s+(or\s+replace\s+)?(materialized\s+)?(view|function|procedure)\b`)
)

// Inbox access is read through member_inbox_access or inbox_viewers, which add teams' grants. Only
// the queries that write a member's own grant and the view's definition name inbox_members.
func TestNoRawInboxMembersReads(t *testing.T) {
	allowedQueries := []string{"GrantInboxAccess", "RevokeInboxAccess"}
	const viewMigration = "00101_teams.sql"
	const ownerRowsMigration = "00135_personal_mailboxes.sql"

	files, err := filepath.Glob("queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no queries: %v", err)
	}
	name := regexp.MustCompile(`^(\w+)`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range strings.Split(string(b), "-- name: ")[1:] {
			n := name.FindString(q)
			body := sqlComment.ReplaceAllString(q, "")
			if inboxMembersRef.MatchString(body) && !slices.Contains(allowedQueries, n) {
				t.Errorf("%s: %s names inbox_members; read member_inbox_access or inbox_viewers instead", filepath.Base(f), n)
			}
		}
	}

	migrations, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	seenView := false
	for _, f := range migrations {
		base := filepath.Base(f)
		if base < viewMigration {
			continue
		}
		if base == viewMigration {
			seenView = true
			continue
		}
		if base == ownerRowsMigration {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		body := sqlComment.ReplaceAllString(string(b), "")
		if viewOrFunction.MatchString(body) && inboxMembersRef.MatchString(body) {
			t.Errorf("%s: a view or function names inbox_members; build it on member_inbox_access", base)
		}
	}
	if !seenView {
		t.Fatalf("migration %s with the view is gone; update this test", viewMigration)
	}

	err = filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".gen.go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && inboxMembersRef.MatchString(s) {
				t.Errorf("%s: a string names inbox_members; read member_inbox_access or inbox_viewers instead", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
