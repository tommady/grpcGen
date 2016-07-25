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
	Messages    map[string][]*MsgMember
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
		msgs := make(map[string][]*MsgMember)
		srvs := make(map[string][]*SrvFunc)
		outPath, err := getOutPath(inPath)
		if err != nil {
			log.Fatalln(err)
		}
		f, err := fetchAstFileFromPath(inPath)
		if err != nil {
			log.Println(err)
		}
		if len(f.Decls) == 0 {
			createProtoFile(outPath, f.Name.Name, msgs, srvs)
			outExampleOnSource(inPath)
			log.Fatalln("no declaration exists, going to generate example")
		}
		for i, decl := range f.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				if genDecl.Doc == nil {
					continue
				}
				if msg, err := fetchMsg(genDecl); err == nil {
					if msg != nil {
						msgs[msg.Name] = msg.Members
					}
				} else {
					log.Printf("decl[%d] fetchMsg fail:%q", i, err)
				}
			} else if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if funcDecl.Doc == nil {
					continue
				}
				if srv, err := fetchSrv(funcDecl); err == nil {
					if srv != nil {
						srvs[srv.Name] = append(srvs[srv.Name], srv.Funcs)
					}
				} else {
					log.Printf("decl[%d] fetchSrv fail:%q", i, err)
				}
			} else {
				log.Printf("decl[%d] cannot be converted into FuncDecl or genDecl", i)
			}
		}
		if len(msgs) == 0 {
			log.Fatalf("%s symbol cannot be found", msgSymbol)
		}
		if len(srvs) == 0 {
			log.Fatalf("%s symbol cannot be found", srvSymbol)
		}
		if err := correctTypes(msgs); err != nil {
			log.Fatalln(err)
		}
		if err := createProtoFile(outPath, f.Name.Name, msgs, srvs); err != nil {
			log.Fatalln(err)
		}
		if err := callProtoc(outPath); err != nil {
			log.Fatalln(err)
		}
		if err := markMsgAsComment(inPath); err != nil {
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
	return nil, nil
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
	if foundSrv && foundSrvName {
		return srv, nil
	}
	return nil, nil
}

func getOutPath(path string) (string, error) {
	if !strings.HasSuffix(path, ".go") {
		return "", fmt.Errorf("path %s doesn't have .go extension", path)
	}
	trimmed := strings.TrimSuffix(path, ".go")
	_, file := filepath.Split(trimmed)
	absDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(absDir, "/pb", fmt.Sprintf("%s.go.proto", file)), nil
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

func createProtoFile(path, packageName string, msgs map[string][]*MsgMember, srvs map[string][]*SrvFunc) error {
	// "/pb" foder does not exist, create it
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		if err := os.Mkdir("pb", 0777); err != nil {
			return err
		}
	}
	// protobuf file existed, delete it
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0777)
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
	outFolder := fmt.Sprintf("--go_out=plugins=grpc:%s", dir)
	args := []string{"-I", dir, path, outFolder}
	cmdProc := exec.Command(cmd, args...)
	var stderr bytes.Buffer
	cmdProc.Stderr = &stderr
	if err := cmdProc.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}
	return nil
}

func outExampleOnSource(path string) error {
	if path == "" {
		return fmt.Errorf("File does not exist")
	}
	if !strings.HasSuffix(path, ".go") {
		return fmt.Errorf("path %s doesn't have .go extension", path)
	}
	in, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(in), "\n")
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	exLines := strings.Split(getExampleText(strings.Split(dir, "/src/")[1]), "\n")
	lines = append(lines, exLines...)
	out := strings.Join(lines, "\n")
	err = ioutil.WriteFile(path, []byte(out), 0644)
	if err != nil {
		return nil
	}
	return nil
}

// correctTypes follows gRPC Scalar Value Types to do translating:
// https://developers.google.com/protocol-buffers/docs/proto3#scalar
func correctTypes(msgs map[string][]*MsgMember) error {
	if msgs == nil {
		return fmt.Errorf("input msgs is nil")
	}
	// TODO: converting types needs to be specified in individual,
	// like []map[string]int type or []int ... etc all needs to be handled.
	for _, v := range msgs {
		for _, msg := range v {
			if msg.Type == "int" {
				msg.Type = "int32"
			} else if msg.Type == "uint" {
				msg.Type = "uint32"
			} else if msg.Type == "[]byte" {
				msg.Type = "bytes"
			} else if strings.HasPrefix(msg.Type, "[]") {
				t := strings.TrimPrefix(msg.Type, "[]")
				msg.Type = "repeated " + t
			} else if strings.HasPrefix(msg.Type, "map") {
				t := strings.TrimPrefix(msg.Type, "map")
				t = strings.Map(func(r rune) rune {
					switch {
					case r == '[':
						return ' '
					case r == ']':
						return ' '
					case r == '*':
						return ' '
					}
					return r
				}, t)
				ts := strings.Fields(t)
				msg.Type = fmt.Sprintf("map<%s, %s>", ts[0], ts[1])
			}
			if strings.Contains(msg.Type, "interface{}") {
				msg.Type = strings.Replace(
					msg.Type,
					"interface{}",
					"google.protobuf.Value",
					-1,
				)
			}
			if strings.Contains(msg.Type, "*") {
				msg.Type = strings.Replace(msg.Type, "*", "", -1)
			}
		}
	}
	return nil
}

func getTemplateText() string {
	return `//
// Generated by grpcGen -- DO NOT EDIT
//
syntax = "proto3";

package {{ .PackageName }}_pb;

import "google/protobuf/struct.proto";
{{ range $key, $value := .Services }}
service {{ $key }} {
  {{ range $value }}
  rpc {{ .Name }} ({{ .In }}) returns ({{ .Out }}) {}
  {{ end }}
}
{{ end }}

{{ range $key, $value := .Messages }}
message {{ $key }} {
  {{ range $index, $member := $value }}
  {{ $member.Type }} {{ $member.Name }} = {{ add $index 1 }};
  {{ end }}
}
{{ end }}`
}

func getExampleText(path string) string {
	return fmt.Sprintf(`import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pb "%s/pb"
)

// @grpcGen:Message
type Request struct {
	InEditMe1    string
}

// @grpcGen:Message
type Reply struct {
	OutEditMe1    string
}

// @grpcGen:Service
// @grpcGen:SrvName: EditMe
func (q *server) FuncEditMe1(ctx context.Context, in *pb.Request) (out *pb.Reply, err error) {
	return &pb.Reply{OutEditMe1: "Hello " + in.InEditMe1}, nil
}

// @grpcGen:Service
//@grpcGen:SrvName: EditMe
func (s *server) FuncEditMe2(ctx context.Context, in *pb.Request) (out *pb.Reply, err error) {
	return &pb.Reply{OutEditMe1: "Hey " + in.InEditMe1}, nil
}`, path)
}
