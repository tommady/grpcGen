package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	msgSymbol      = "@grpcGen:Message"
	srvSymbol      = "@grpcGen:Service"
	srvNameSymbol  = "@grpcGen:SrvName:"
	srvParamSymbol = "*pb."
	srvRetSymbol   = "*pb."
)

// OutTemplatData is the layout proto file template's data.
type OutTemplatData struct {
	PackageName string
	Messages    []*Msg
	Services    []*Srv
}

// Msg stands for every declaration in gRPC Message type.
type Msg struct {
	Name    string
	Members []*MsgMember
}

// MsgMember is the gRPC Message type's member.
type MsgMember struct {
	Name string
	Type string
}

// Srv stands for every RPC mapping function in gRPC Service type.
type Srv struct {
	Name  string
	Funcs []*SrvFunc
}

// SrvFunc is the gRPC Service type's function member.
type SrvFunc struct {
	Name string
	In   string
	Out  string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("grpcGen: ")
	for _, inPath := range os.Args[1:] {
		msgs := []*Msg{}
		srvs := []*Srv{}
		f, err := fetchAstFileFromPath(inPath)
		if err != nil {
			log.Println(err)
		}
		if len(f.Decls) == 0 {
			log.Fatalln("no declaration exists")
		}
		for _, decl := range f.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				if msg, ok := fetchMsg(genDecl); ok {
					msgs = append(msgs, msg)
				}
			}
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if srv, ok := fetchSrv(funcDecl); ok {
					srvs = append(srvs, srv)
				}
			}
		}
		outPath, err := getOutPath(inPath)
		if err != nil {
			log.Fatalln(err)
		}
		if _, err = os.Stat(outPath); err == nil {
			err = os.Remove(outPath)
			if err != nil {
				log.Fatalln(err)
			}
		}
		outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			log.Fatalln(err)
		}
		defer outFile.Close()
		outData := new(OutTemplatData)
		outData.Messages = msgs
		outData.Services = srvs
		outData.PackageName = f.Name.Name
		tmplFuncs := template.FuncMap{"add": func(x, y int) int { return x + y }}
		outTmpl := template.Must(template.New("outProto").Funcs(tmplFuncs).Parse(getTemplateText()))
		if err := outTmpl.Execute(outFile, outData); err != nil {
			log.Fatalln(err)
		}
	}
}

// fetchAstFileFromPath returns a ast file if input path is valid.
func fetchAstFileFromPath(path string) (*ast.File, error) {
	if path == "" {
		return nil, fmt.Errorf("File does not exist.")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func fetchMsg(genDecl *ast.GenDecl) (*Msg, bool) {
	if genDecl.Doc == nil {
		return nil, false
	}
	msg := new(Msg)
	found := false
	for _, comment := range genDecl.Doc.List {
		if found {
			break
		}
		if strings.Contains(comment.Text, msgSymbol) {
			found = true
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil {
					return nil, false
				}
				msg.Name = typeSpec.Name.Name
				struc := typeSpec.Type.(*ast.StructType)
				for _, s := range struc.Fields.List {
					memb := new(MsgMember)
					memb.Type = types.ExprString(s.Type)
					for _, name := range s.Names {
						if name != nil {
							memb.Name = name.Name
						}
					}
					msg.Members = append(msg.Members, memb)
				}
			}
		}
	}
	if found {
		return msg, true
	}
	return nil, false
}

func fetchSrv(funcDecl *ast.FuncDecl) (*Srv, bool) {
	if funcDecl.Doc == nil {
		return nil, false
	}
	srv := new(Srv)
	foundSrv := false
	foundSrvName := false
	for _, comment := range funcDecl.Doc.List {
		if foundSrv && foundSrvName {
			break
		}
		if strings.Contains(comment.Text, srvSymbol) {
			foundSrv = true
			fun := new(SrvFunc)
			fun.Name = funcDecl.Name.Name
			for _, param := range funcDecl.Type.Params.List {
				strType := types.ExprString(param.Type)
				if strings.Contains(strType, srvParamSymbol) {
					fun.In = strings.TrimPrefix(strType, srvParamSymbol)

				}
			}
			for _, ret := range funcDecl.Type.Results.List {
				strType := types.ExprString(ret.Type)
				if strings.Contains(strType, srvParamSymbol) {
					fun.Out = strings.TrimPrefix(strType, srvParamSymbol)

				}
			}
			srv.Funcs = append(srv.Funcs, fun)
		} else if strings.Contains(comment.Text, srvNameSymbol) {
			foundSrvName = true
			if srv.Name == "" {
				srv.Name = strings.TrimPrefix(comment.Text, "// "+srvNameSymbol)
				srv.Name = strings.TrimPrefix(srv.Name, "//"+srvNameSymbol)
				srv.Name = strings.Trim(srv.Name, " ")
			}
		}
	}
	if foundSrvName && foundSrv {
		return srv, true
	}
	return nil, false
}

func getOutPath(path string) (string, error) {
	if !strings.HasSuffix(path, ".go") {
		return "", fmt.Errorf("path %s doesn't have .go extension", path)
	}
	trimmed := strings.TrimSuffix(path, ".go")
	dir, file := filepath.Split(trimmed)
	return filepath.Join(dir, fmt.Sprintf("%s.go.proto", file)), nil
}

func getTemplateText() string {
	return `//
// Generated by grpcGen -- DO NOT EDIT
//
syntax = "proto3";

package {{ .PackageName }};
{{ range .Services }}
service {{ .Name }} {
  {{ range .Funcs }}
  rpc {{ .Name }} ({{ .In }}) returns ({{ .Out }}) {}
  {{ end }}
}
{{ end }}

{{ range .Messages }}
message {{ .Name }} {
  {{ range $index, $member := .Members }}
  {{ $member.Type }} {{ $member.Name }} = {{ add $index 1 }};
  {{ end }}
}
{{ end }}`
}