package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func call(args []string, stdin string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestCommandLine(t *testing.T) {
	code, out, errs := call([]string{"../../scenes/terrace-street.json"}, "")
	if code != 0 || !strings.Contains(out, "SHADEGROVE  Terrace main street") || !strings.Contains(out, "at least") {
		t.Fatalf("report: %d %s %s", code, out, errs)
	}
	code, out, _ = call([]string{"-json", "-"}, readFile(t, "../../scenes/town-square.json"))
	var res map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res["plan"] == nil {
		t.Fatal("JSON output from standard input")
	}
	for _, args := range [][]string{{}, {"--help"}, {"a", "b"}} {
		if c, _, _ := call(args, ""); c != 2 {
			t.Errorf("%v: usage errors exit 2", args)
		}
	}
	if c, _, e := call([]string{"../../scenes"}, ""); c != 2 || !strings.Contains(e, "could not be read") {
		t.Error("a folder is refused")
	}
	if c, _, e := call([]string{"secret-name-zz.json"}, ""); c != 2 || strings.Contains(e, "secret-name") {
		t.Error("errors don't echo the path")
	}
	if c, _, e := call([]string{"-"}, `{"name": "<script>alert(1)</script>"}`); c != 2 || strings.Contains(e, "<script>") {
		t.Error("errors don't echo the input")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	var b bytes.Buffer
	code := run([]string{"-json", path}, nil, &b, &b)
	if code != 0 {
		t.Fatalf("could not run %s", path)
	}
	// Re-read the scene itself rather than the result.
	var res struct {
		Scene json.RawMessage `json:"scene"`
	}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return string(res.Scene)
}
