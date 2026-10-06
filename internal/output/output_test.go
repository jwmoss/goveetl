package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	var stdout bytes.Buffer
	formatter := New(&stdout, &bytes.Buffer{}, true, false, false, true)
	if err := formatter.JSON(map[string]string{"hello": "world"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"hello": "world"`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestQuietSuppressesTable(t *testing.T) {
	var stdout bytes.Buffer
	formatter := New(&stdout, &bytes.Buffer{}, false, false, true, true)
	formatter.Table([]string{"A"}, [][]string{{"B"}})
	if stdout.String() != "" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestTermDumbDisablesColor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	var stdout bytes.Buffer
	formatter := New(&stdout, &bytes.Buffer{}, false, false, false, false)
	formatter.Success("saved")
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("stdout contains color escape: %q", stdout.String())
	}
}

func TestRawJSONExact(t *testing.T) {
	const raw = `{ "id":9007199254740993, "id":1e100 }`
	var stdout bytes.Buffer
	f := New(&stdout, &bytes.Buffer{}, true, false, false, true)
	if err := f.JSON(json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != raw+"\n" {
		t.Fatalf("raw output = %q", stdout.String())
	}
}
