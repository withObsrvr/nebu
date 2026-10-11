package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/withObsrvr/nebu/pkg/registry"
)

func TestModuleInstallTarget(t *testing.T) {
	tests := []struct {
		name, module, ref, want string
	}{
		{name: "latest", module: "example.com/processors/cmd/foo", want: "example.com/processors/cmd/foo"},
		{name: "tag", module: "example.com/processors/cmd/foo", ref: "v1.2.3", want: "example.com/processors/cmd/foo@v1.2.3"},
		{name: "commit", module: "example.com/processors/cmd/foo", ref: "abc123", want: "example.com/processors/cmd/foo@abc123"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := moduleInstallTarget(test.module, test.ref); got != test.want {
				t.Fatalf("moduleInstallTarget() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrintDescribeShowsDeprecation(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = write
	t.Cleanup(func() { os.Stdout = original })

	proc := &registry.ProcessorEntry{
		Name:              "old-origin",
		Type:              "origin",
		Description:       "Old origin",
		Deprecated:        true,
		DeprecationNotice: "Use new-origin instead",
	}
	if err := printDescribe(proc, nil); err != nil {
		t.Fatalf("printDescribe: %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	output, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	got := string(output)
	for _, want := range []string{"Deprecated: yes", "Deprecation notice: Use new-origin instead"} {
		if !strings.Contains(got, want) {
			t.Fatalf("describe output %q does not contain %q", got, want)
		}
	}
}
