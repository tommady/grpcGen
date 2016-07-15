package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestFetchMsg(t *testing.T) {
	src := `
package grpc_test
// @grpcGen:Message
type Reply struct {
        Name    string
        Email   string
        Counter int32
}`
	expect := &Msg{
		Name: "Reply",
		Members: []*MsgMember{
			{Name: "Name", Type: "string"},
			{Name: "Email", Type: "string"},
			{Name: "Counter", Type: "int32"},
		},
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for i, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			if actual, err := fetchMsg(genDecl); err == nil {
				if !reflect.DeepEqual(expect, actual) {
					t.Errorf("decl[%d] actual and expect are not the same", i)
				}
			} else {
				t.Errorf("decl[%d] fetchMsg: %q", i, err)
			}
		} else {
			t.Errorf("decl[%d] cannot be converted into GenDecl", i)
		}
	}
}

func TestFetchSrv(t *testing.T) {
	src := `
package grpc_test
// @grpcGen:Service
// @grpcGen:SrvName: Greeting
func (q *server) SayHello(ctx context.Context, in *pb.Request) (out *pb.Reply, err error) {
	return &pb.Reply{Message: "Hello " + in.Name}, nil
}`
	expect := &Srv{
		Name: "Greeting",
		Funcs: []*SrvFunc{
			{Name: "SayHello", In: "Request", Out: "Reply"},
		},
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for i, decl := range f.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if actual, err := fetchSrv(funcDecl); err == nil {
				if !reflect.DeepEqual(expect, actual) {
					t.Errorf("decl[%d] actual and expect are not the same", i)
				}
			} else {
				t.Errorf("decl[%d] fetchSrv: %q", i, err)
			}
		} else {
			t.Errorf("decl[%d] cannot be converted into FuncDecl", i)
		}
	}
}
