package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestCorrectTypes(t *testing.T) {
	expects := map[string][]*MsgMember{
		"Test": []*MsgMember{
			{Name: "Age", Type: "uint32"},
			{Name: "Name", Type: "bytes"},
			{Name: "Money", Type: "int32"},
			{Name: "Account", Type: "repeated string"},
			{Name: "TMap", Type: "map<string, Bar>"},
			{Name: "PointerS", Type: "Bar"},
		},
	}
	actuals := map[string][]*MsgMember{
		"Test": []*MsgMember{
			{Name: "Age", Type: "uint"},
			{Name: "Name", Type: "[]byte"},
			{Name: "Money", Type: "int"},
			{Name: "Account", Type: "[]string"},
			{Name: "TMap", Type: "map[string]*Bar"},
			{Name: "PointerS", Type: "*Bar"},
		},
	}
	err := correctTypes(actuals)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expects, actuals) {
		t.Errorf("actual and expect are not the same")
	}
}

func TestFetchMsg(t *testing.T) {
	src := `
package grpc_test
// @grpcGen:Message
type Reply struct {
        Name    string
        Email   string
        Counter int32
}`
	expect := []*Msg{
		{
			Name: "Reply",
			Members: []*MsgMember{
				{Name: "Name", Type: "string"},
				{Name: "Email", Type: "string"},
				{Name: "Counter", Type: "int32"},
			},
		},
	}
	actual := []*Msg{}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for i, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			if msg, err := fetchMsg(genDecl); err == nil {
				actual = append(actual, msg)
			} else {
				t.Errorf("decl[%d] fetchMsg: %q", i, err)
			}
		} else {
			t.Errorf("decl[%d] cannot be converted into GenDecl", i)
		}
	}
	if !reflect.DeepEqual(expect, actual) {
		t.Errorf("actual and expect are not the same")
	}
}

func TestFetchSrv(t *testing.T) {
	src := `
package grpc_test
// @grpcGen:Service
// @grpcGen:SrvName: Greeting
func (q *server) SayHello(ctx context.Context, in *pb.Request) (out *pb.Reply, err error) {
	return &pb.Reply{Message: "Hello " + in.Name}, nil
}
// @grpcGen:Service
// @grpcGen:SrvName: Greeting
func (q *server) SayYa(ctx context.Context, in *pb.Request) (out *pb.Reply, err error) {
	return &pb.Reply{Message: "Ya " + in.Name}, nil
}`
	expect := map[string][]*SrvFunc{
		"Greeting": []*SrvFunc{
			{Name: "SayHello", In: "Request", Out: "Reply"},
			{Name: "SayYa", In: "Request", Out: "Reply"},
		},
	}
	actual := make(map[string][]*SrvFunc)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for i, decl := range f.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if srv, err := fetchSrv(funcDecl); err == nil {
				actual[srv.Name] = append(actual[srv.Name], srv.Funcs)
			} else {
				t.Errorf("decl[%d] fetchSrv: %q", i, err)
			}
		} else {
			t.Errorf("decl[%d] cannot be converted into FuncDecl", i)
		}
	}
	if !reflect.DeepEqual(expect, actual) {
		t.Errorf("actual and expect are not the same")
	}
}
