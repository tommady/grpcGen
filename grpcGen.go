package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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
	Services    map[string][]*SrvFunc
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
	Funcs *SrvFunc
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
		srvs := make(map[string][]*SrvFunc)
		f, err := fetchAstFileFromPath(inPath)
		if err != nil {
			log.Println(err)
		}
		if len(f.Decls) == 0 {
			log.Fatalln("no declaration exists")
		}
		for i, decl := range f.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				if msg, err := fetchMsg(genDecl); err == nil {
					msgs = append(msgs, msg)
				} else {
					log.Printf("decl[%d] fetchMsg fail:%q", i, err)
				}
			} else if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if srv, err := fetchSrv(funcDecl); err == nil {
					srvs[srv.Name] = append(srvs[srv.Name], srv.Funcs)
				} else {
					log.Printf("decl[%d] fetchSrv fail:%q", i, err)
				}
			} else {
				log.Printf("decl[%d] cannot be converted into FuncDecl or genDecl", i)
			}
		}
		if err := markMsgAsComment(inPath); err != nil {
			log.Fatalln(err)
		}
		outPath, err := getOutPath(inPath)
		if err != nil {
			log.Fatalln(err)
		}
		if err := createProtoFile(outPath, f.Name.Name, msgs, srvs); err != nil {
			log.Fatalln(err)
		}
		if err := callProtoc(outPath); err != nil {
			log.Fatalln(err)
		}
	}
}

// fetchAstFileFromPath returns a ast file if input path is valid.
func fetchAstFileFromPath(path string) (*ast.File, error) {
	if path == "" {
		return nil, fmt.Errorf("File does not exist")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func fetchMsg(genDecl *ast.GenDecl) (*Msg, error) {
	if genDecl.Doc == nil {
		return nil, fmt.Errorf("genDecl.Doc is nil")
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
				if !ok {
					return nil, fmt.Errorf("fail to convert into ast.TypeSpec")
				}
				if typeSpec.Name == nil {
					return nil, fmt.Errorf("typeSpec.Name is nil")
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
		return msg, nil
	}
	return nil, fmt.Errorf("%s symbol cannot be found", msgSymbol)
}

func fetchSrv(funcDecl *ast.FuncDecl) (*Srv, error) {
	if funcDecl.Doc == nil {
		return nil, fmt.Errorf("funcDecl.Doc is nil")
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
			srv.Funcs = fun
		} else if strings.Contains(comment.Text, srvNameSymbol) {
			foundSrvName = true
			if srv.Name == "" {
				srv.Name = strings.TrimPrefix(comment.Text, "// "+srvNameSymbol)
				srv.Name = strings.TrimPrefix(srv.Name, "//"+srvNameSymbol)
				srv.Name = strings.Trim(srv.Name, " ")
			}
		}
	}
	if !foundSrvName {
		return nil, fmt.Errorf("%s symbol cannot be found", srvNameSymbol)
	} else if !foundSrv {
		return nil, fmt.Errorf("%s symbol cannot be found", srvSymbol)
	}
	return srv, nil
}

func getOutPath(path string) (string, error) {
	if !strings.HasSuffix(path, ".go") {
		return "", fmt.Errorf("path %s doesn't have .go extension", path)
	}
	trimmed := strings.TrimSuffix(path, ".go")
	dir, file := filepath.Split(trimmed)
	return filepath.Join(dir, fmt.Sprintf("%s.go.proto", file)), nil
}

func markMsgAsComment(path string) error {
	if path == "" {
		return fmt.Errorf("File does not exist")
	}
	in, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(in), "\n")
	for i := 0; i < len(lines); i++ {
		if strings.Contains(lines[i], msgSymbol) {
			for j := i + 1; ; j++ {
				if !strings.HasPrefix(lines[j], "//") {
					lines[j] = "// " + lines[j]
				}
				if strings.Contains(lines[j], "}") {
					i = j
					break
				}
			}
		}
	}
	out := strings.Join(lines, "\n")
	err = ioutil.WriteFile(path, []byte(out), 0644)
	if err != nil {
		return nil
	}
	return nil
}

func createProtoFile(path, packageName string, msgs []*Msg, srvs map[string][]*SrvFunc) error {
	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		if err != nil {
			return err
		}
	}
	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer outFile.Close()
	outData := new(OutTemplatData)
	outData.Messages = msgs
	outData.Services = srvs
	outData.PackageName = packageName
	tmplFuncs := template.FuncMap{"add": func(x, y int) int { return x + y }}
	outTmpl := template.Must(template.New("outProto").Funcs(tmplFuncs).Parse(getTemplateText()))
	if err := outTmpl.Execute(outFile, outData); err != nil {
		return err
	}
	return nil
}

func callProtoc(path string) error {
	if !strings.HasSuffix(path, ".go.proto") {
		return fmt.Errorf("path %s doesn't have .go.proto extension", path)
	}
	trimmed := strings.TrimSuffix(path, ".go.proto")
	dir, _ := filepath.Split(trimmed)
	cmd := "protoc"
	args := []string{"-I", dir, path, "--go_out=plugins=grpc:."}
	cmdProc := exec.Command(cmd, args...)
	var stderr bytes.Buffer
	cmdProc.Stderr = &stderr
	if err := cmdProc.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}
	return nil
}

func getTemplateText() string {
	return `//
// Generated by grpcGen -- DO NOT EDIT
//
syntax = "proto3";

package {{ .PackageName }};
{{ range $key, $value := .Services }}
service {{ $key }} {
  {{ range $value }}
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
