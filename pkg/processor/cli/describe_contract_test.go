package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// describe.go calls --describe-json "the canonical flag every CLI helper
// registers". That was not true: RunGenericOriginCLI and RunOriginCLI never
// registered it, so a processor built on either answered
//
//	Error: unknown flag: --describe-json
//
// while sinks, transforms and proto origins answered correctly. It is not a
// cosmetic gap — docs/STABILITY.md lists the describe protocol as a stable
// surface, and nebu shells out to it to install a processor at all. The one
// registry processor on the generic helper had hand-rolled its own describe
// command to compensate, which is how the hole stayed hidden: the processor
// that would have exposed it had already worked around it.
//
// The test reads the source rather than running the helpers. Each Run*CLI
// builds its own cobra command, executes it, and calls os.Exit, so there is
// no command to introspect and no way to call one from a test without
// terminating the test binary. What can be checked is that every helper
// contains both halves of the contract.
func TestEveryCLIHelperRegistersDescribeJSON(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}

	helpers := map[string]*ast.FuncDecl{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || fn.Body == nil {
					continue
				}
				// The public entry points: RunOriginCLI, RunProtoOriginCLI,
				// RunGenericOriginCLI, RunTransformCLI, RunSinkCLI.
				if strings.HasPrefix(fn.Name.Name, "Run") && strings.HasSuffix(fn.Name.Name, "CLI") {
					helpers[fn.Name.Name] = fn
				}
			}
		}
	}

	if len(helpers) < 5 {
		t.Fatalf("found %d Run*CLI helpers (%v); expected at least the five that exist",
			len(helpers), namesOf(helpers))
	}

	for name, fn := range helpers {
		t.Run(name, func(t *testing.T) {
			var registers, emits bool
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.Ident:
					if n.Name == "describeFlagName" {
						registers = true
					}
				case *ast.CallExpr:
					if ident, ok := n.Fun.(*ast.Ident); ok && ident.Name == "emitDescribeIfRequested" {
						emits = true
					}
				}
				return true
			})

			if !registers {
				t.Errorf("%s does not register the describe-json flag", name)
			}
			if !emits {
				// Registering without the pre-cobra short-circuit is worse
				// than not registering at all: the flag appears in --help
				// and then fails on any processor with a required flag.
				t.Errorf("%s does not call emitDescribeIfRequested, so --describe-json "+
					"would run cobra's required-flag validation first", name)
			}
		})
	}
}

func namesOf(helpers map[string]*ast.FuncDecl) []string {
	names := make([]string, 0, len(helpers))
	for name := range helpers {
		names = append(names, name)
	}
	return names
}
