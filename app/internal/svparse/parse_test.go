package svparse

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseValues(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want map[string]any
	}{
		{"int", "A = 12", map[string]any{"A": int64(12)}},
		{"negative int", "A = -7", map[string]any{"A": int64(-7)}},
		{"float", "A = 0.512", map[string]any{"A": 0.512}},
		{"negative float", "A = -1.5", map[string]any{"A": -1.5}},
		{"exponent is float", "A = 1e3", map[string]any{"A": 1000.0}},
		{"bools", "A = true\nB = false", map[string]any{"A": true, "B": false}},
		{"nil global", "A = nil", map[string]any{"A": nil}},
		{"string escapes", `A = "a\"b\\c\nd\065"`, map[string]any{"A": "a\"b\\c\nd" + "A"}},
		{"string with ]]", `A = "x]]y"`, map[string]any{"A": "x]]y"}},
		{"long string with ]]", "A = [==[x]]y]==]", map[string]any{"A": "x]]y"}},
		{"comments", "-- hi\nA = 1 -- tail\n--[[ block\n]] B = 2", map[string]any{"A": int64(1), "B": int64(2)}},
		{"array positional", "A = { 1, 2, 3, }", map[string]any{"A": []any{int64(1), int64(2), int64(3)}}},
		{"array explicit keys", "A = { [1] = \"a\", [2] = \"b\" }", map[string]any{"A": []any{"a", "b"}}},
		{"array with index comments", "A = {\n\"x\", -- [1]\n\"y\", -- [2]\n}", map[string]any{"A": []any{"x", "y"}}},
		{"map", `A = { ["k"] = 1, n = true }`, map[string]any{"A": map[string]any{"k": int64(1), "n": true}}},
		{"sparse int keys become map", "A = { [1] = 1, [3] = 3 }", map[string]any{"A": map[string]any{"1": int64(1), "3": int64(3)}}},
		{"empty table is empty slice", "A = {}", map[string]any{"A": []any{}}},
		{"nested", `A = { { ["x"] = { 1 } } }`, map[string]any{"A": []any{map[string]any{"x": []any{int64(1)}}}}},
		{"semicolon separators", "A = { 1; 2 }", map[string]any{"A": []any{int64(1), int64(2)}}},
		{"BOM and CRLF", "\xef\xbb\xbfA = {\r\n\t[\"k\"] = \"v\",\r\n}\r\n", map[string]any{"A": map[string]any{"k": "v"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, src, wantLine string
	}{
		{"truncated table", "A = {\n\t[\"k\"] = 1,\n\t[\"j\"] = {\n", "riga 4"},
		{"truncated string", "A = 1\nB = \"abc", "riga 2"},
		{"missing value", "A =\n", "riga"},
		{"garbage", "A = 1\n\n@", "riga 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.src))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantLine) {
				t.Fatalf("error %q does not contain %q", err, tc.wantLine)
			}
		})
	}
}
