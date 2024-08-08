package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"golang.org/x/tools/go/packages"
)

func createTestAppPackage(testAppDir, fileName, contents string) ([]*decorator.Package, error) {
	err := os.Mkdir(testAppDir, 0755)
	if err != nil {
		return nil, err
	}

	filepath := filepath.Join(testAppDir, fileName)

	f, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	_, err = f.WriteString(contents)
	if err != nil {
		return nil, err
	}

	return decorator.Load(&packages.Config{Dir: testAppDir, Mode: loadMode})
}

func cleanupTestApp(t *testing.T, appDirectoryName string) {
	err := os.RemoveAll(appDirectoryName)
	if err != nil {
		t.Logf("Failed to cleanup test app directory %s: %v", appDirectoryName, err)
	}
}

func Test_isNetHttpClient(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		lineNum int
		want    bool
	}{
		{
			name: "define_new_http_client",
			code: `
package main
import "net/http"
func main() {
	client := &http.Client{}
}`,
			lineNum: 0,
			want:    true,
		},
		{
			name: "define_complex_http_client",
			code: `
package main
import "net/http"
func main() {
	client := &http.Client{
		Timeout: time.Second,
	}
}`,
			lineNum: 0,
			want:    true,
		},
		{
			name: "assign_http_client",
			code: `
package main
import "net/http"
func main() {
	client = &http.Client{}
}`,
			lineNum: 0,
			want:    false,
		},
		{
			name: "reassign_http_client",
			code: `
package main
import "net/http"
func main() {
	client := &http.Client{}
	client2 := client
}`,
			lineNum: 1,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testAppDir := "tmp"
			fileName := tt.name + ".go"
			pkgs, err := createTestAppPackage(testAppDir, fileName, tt.code)
			defer cleanupTestApp(t, testAppDir)
			if err != nil {
				t.Fatal(err)
			}

			decl, ok := pkgs[0].Syntax[0].Decls[1].(*dst.FuncDecl)
			if !ok {
				t.Fatal("code must contain only one function declaration")
			}

			stmt, ok := decl.Body.List[tt.lineNum].(*dst.AssignStmt)
			if !ok {
				t.Fatal("lineNum must point to an assignment statement")
			}

			if got := isNetHttpClientDefinition(stmt); got != tt.want {
				t.Errorf("isNetHttpClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isNetHttpMethodCannotInstrument(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		lineNum      int
		wantBool     bool
		wantFuncName string
	}{
		{
			name: "http_get",
			code: `
package main
import "net/http"
func main() {
	http.Get("http://example.com")
}`,
			lineNum:      0,
			wantBool:     true,
			wantFuncName: "Get",
		},
		{
			name: "http_post",
			code: `
package main
import "net/http"
func main() {
	http.Post("http://example.com")
}`,
			lineNum:      0,
			wantBool:     true,
			wantFuncName: "Post",
		},
		{
			name: "http_post_form",
			code: `
package main
import "net/http"
func main() {
	http.PostForm("http://example.com")
}`,
			lineNum:      0,
			wantBool:     true,
			wantFuncName: "PostForm",
		},
		{
			name: "http_head",
			code: `
package main
import "net/http"
func main() {
	http.Head("http://example.com")
}`,
			lineNum:      0,
			wantBool:     true,
			wantFuncName: "Head",
		},
		{
			name: "http_client_get",
			code: `
package main
import "net/http"
func main() {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	client.Get("https://example.com")
}`,
			lineNum:      2,
			wantBool:     false,
			wantFuncName: "",
		},
		{
			name: "http_client_do",
			code: `
package main
import "net/http"
func main() {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	client.Do(req)
}`,
			lineNum:      2,
			wantBool:     false,
			wantFuncName: "",
		},
		{
			name: "http_get_complex_line",
			code: `
package main
import "net/http"
func main() {
	_, err := http.Get("http://example.com"); if err != nil {
		panic(err)
	}
}`,
			lineNum:      0,
			wantBool:     true,
			wantFuncName: "Get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testAppDir := "tmp"
			fileName := tt.name + ".go"
			pkgs, err := createTestAppPackage(testAppDir, fileName, tt.code)
			defer cleanupTestApp(t, testAppDir)
			if err != nil {
				t.Fatal(err)
			}

			decl, ok := pkgs[0].Syntax[0].Decls[1].(*dst.FuncDecl)
			if !ok {
				t.Fatal("code must contain only one function declaration")
			}

			gotFuncName, gotBool := isNetHttpMethodCannotInstrument(decl.Body.List[tt.lineNum])
			if gotBool != tt.wantBool {
				t.Errorf("isNetHttpMethodCannotInstrument() = %v, want %v", gotBool, tt.wantBool)
			}
			if gotFuncName != tt.wantFuncName {
				t.Errorf("isNetHttpMethodCannotInstrument() = %v, want %v", gotFuncName, tt.wantFuncName)
			}
		})
	}
}
