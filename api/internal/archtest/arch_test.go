// Package archtest enforces the architecture the access model depends on.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChunksOnlyReadInsideRetrievalAndIngest fails the build if any package other
// than retrieve, ingest or the migrations mentions the chunks table. Answering
// can then only get passages through retrieve, whose single query carries the
// team constraint.
func TestChunksOnlyReadInsideRetrievalAndIngest(t *testing.T) {
	root := filepath.Join("..", "..") // the api module root
	allowed := []string{
		filepath.Join("internal", "retrieve"),
		filepath.Join("internal", "ingest"),
		filepath.Join("internal", "db", "migrations"),
		filepath.Join("internal", "archtest"),
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql")) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, a := range allowed {
			if strings.HasPrefix(rel, a+string(filepath.Separator)) {
				return nil
			}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "document_chunks") {
			t.Errorf("%s mentions document_chunks; only internal/retrieve and internal/ingest may touch chunks", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// parseDir parses the non-test Go files of one package directory.
func parseDir(t *testing.T, rel string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	dir := filepath.Join("..", "..", rel)
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = f
	}
	if len(files) == 0 {
		t.Fatalf("no Go files found in %s: the test would check nothing", rel)
	}
	return fset, files
}

// TestEveryRetrievalStatementCarriesTheAccessPredicate reads the real retrieval
// package. Each top-level declaration whose SQL mentions the chunks table, or
// reads documents, must build its WHERE clause from the shared access predicate
// (readableBy or its negation unreadableBy). A new query added to retrieve
// without the team constraint fails here, whatever else it does.
func TestEveryRetrievalStatementCarriesTheAccessPredicate(t *testing.T) {
	_, files := parseDir(t, filepath.Join("internal", "retrieve"))
	checked := 0
	for name, f := range files {
		for _, decl := range f.Decls {
			var touchesData, usesPredicate bool
			ast.Inspect(decl, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.BasicLit:
					if x.Kind == token.STRING && (strings.Contains(x.Value, "document_chunks") || strings.Contains(x.Value, "FROM documents")) {
						touchesData = true
					}
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok && (id.Name == "readableBy" || id.Name == "unreadableBy") {
						usesPredicate = true
					}
				}
				return true
			})
			if !touchesData {
				continue
			}
			checked++
			// The predicate helpers themselves hold the EXISTS text, not a query.
			if !usesPredicate {
				t.Errorf("internal/retrieve/%s: a declaration reads documents or chunks without readableBy/unreadableBy", name)
			}
		}
	}
	if checked < 3 {
		t.Fatalf("only %d retrieval statements were found; expected the search, the probe and the document read", checked)
	}
}

// TestAnswerCannotReachTheDatabase keeps answer.Generate fed by retrieve alone:
// the answer package may import retrieve (for the Permitted type) and nothing that
// can query, so the only passages it can ever see are the ones handed to it.
func TestAnswerCannotReachTheDatabase(t *testing.T) {
	banned := []string{"github.com/jackc/pgx", "database/sql", "keystone/internal/store", "keystone/internal/ingest", "keystone/internal/db"}
	_, files := parseDir(t, filepath.Join("internal", "answer"))
	sawRetrieve := false
	for name, f := range files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "keystone/internal/retrieve" {
				sawRetrieve = true
			}
			for _, b := range banned {
				if strings.HasPrefix(path, b) {
					t.Errorf("internal/answer/%s imports %s; the answer package must not be able to query", name, path)
				}
			}
		}
	}
	if !sawRetrieve {
		t.Error("internal/answer no longer imports retrieve; Generate must take retrieve.Permitted")
	}
}

// TestRetrieveDoesNotDependOnAnswerOrTheHTTPLayer keeps the dependency pointing
// one way: the HTTP layer wires retrieve to answer, and retrieve knows nothing of
// either, so there is no path from the scoped query back around itself.
func TestRetrieveDoesNotDependOnAnswerOrTheHTTPLayer(t *testing.T) {
	_, files := parseDir(t, filepath.Join("internal", "retrieve"))
	for name, f := range files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "keystone/internal/answer") || strings.HasPrefix(path, "keystone/internal/httpapi") {
				t.Errorf("internal/retrieve/%s imports %s", name, path)
			}
		}
	}
}
