// Test Utils contains tools and building blocks that can be generically used for unit tests

package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

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

func panicRecovery(t *testing.T) {
	err := recover()
	if err != nil {
		t.Fatalf("%s recovered from panic: %+v\n\n%s", t.Name(), err, debug.Stack())
	}
}
