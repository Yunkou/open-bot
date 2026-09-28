package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestUserAndAgentSQLRespectsSoftDelete fails when a function queries or updates
// users/agents without "deleted_at IS NULL" and without a soft-delete: exception comment.
func TestUserAndAgentSQLRespectsSoftDelete(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	dml := regexp.MustCompile(`(?i)\b(?:from|update|join|delete\s+from)\s+(?:users|agents)\b`)
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			src, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				start := fset.Position(fn.Pos()).Offset
				end := fset.Position(fn.End()).Offset
				body := string(src[start:end])
				if fn.Doc != nil {
					body = fn.Doc.Text() + "\n" + body
				}
				if !dml.MatchString(body) {
					continue
				}
				if strings.Contains(body, "soft-delete:") || strings.Contains(body, "deleted_at IS NULL") {
					continue
				}
				t.Errorf("%s %s: users/agents SQL 需要 deleted_at IS NULL，或在函数注释标明 soft-delete: 例外", filename, fn.Name.Name)
			}
		}
	}
}
