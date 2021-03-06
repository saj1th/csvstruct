package csvstruct

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

// Accounts for changes between Go 1.3 and 1.4 that changed how encoding/csv encodes empty strings
// See https://github.com/golang/go/commit/6ad2749dcd614270ac58c5254b6ada3bce0af090
// Set this to true if tests are failing against Go 1.3 or before
const backcompat = false

func TestEncodeNext(t *testing.T) {
	type row struct {
		Foo, Bar, Baz string
	}

	for _, c := range []struct {
		rows []interface{}
		want string
	}{{
		[]interface{}{row{"a", "b", "c"}, row{"d", "e", "f"}},
		`Foo,Bar,Baz
a,b,c
d,e,f
`,
	}, {
		[]interface{}{row{"a", "", ""}, row{"", "b", ""}},
		`Foo,Bar,Baz
a,,
,b,
`,
	}, {
		// Encoding incomplete structs still fills in missing columns.
		[]interface{}{row{Foo: "a"}, struct{ Foo, Bar string }{Bar: "b"}},
		`Foo,Bar,Baz
a,,
,b,
`,
	}, {
		// Encoding unexported fields.
		[]interface{}{struct{ Exported, unexported string }{"a", "b"}},
		`Exported
a
`,
	}, {
		// Encoding renamed and ignored fields.
		[]interface{}{struct {
			Foo     string `csv:"renamed_foo"`
			Bar     string
			Ignored string `csv:"-"`
			Baz     string
		}{"a", "b", "c", "d"}},
		`renamed_foo,Bar,Baz
a,b,d
`,
	}, {
		// If the first row contains no encodable fields, further rows
		// will be ignored as well, resulting in an empty output.
		[]interface{}{struct {
			Ignored    string `csv:"-"`
			unexported string
		}{"you", "won't"}, struct {
			Exported string
		}{"see"}},
		"",
	}, {
		// Encoding non-string fields.
		[]interface{}{struct {
			Int     int
			Int64   int64
			Uint64  uint64
			Float64 float64
			Bool    bool
		}{123, -123456789, 123456789, 123.456, true}},
		`Int,Int64,Uint64,Float64,Bool
123,-123456789,123456789,123.456000,true
`,
	}, {
		// Encoding rows with different fields.
		// Headers are taken from the fields in the first call to EncodeNext.
		// Further calls add whatever fields they can, and if no fields are
		// shared then the row is not written.
		[]interface{}{
			struct{ Foo, Bar string }{"foo", "bar"},
			struct{ Baz string }{"baz"},              // Will be skipped because it shares no fields.
			struct{ Bar, Baz string }{"bar", "baz"}}, // Only shares Bar, only writes Bar.
		`Foo,Bar
foo,bar
,bar
`,
	}, {
		// Encoding rows with the same fields but with different types.
		[]interface{}{
			struct{ Foo string }{"foo"},
			struct{ Foo int64 }{123},
			struct{ Foo bool }{true}},
		`Foo
foo
123
true
`,
	}} {
		var buf bytes.Buffer
		e := NewEncoder(&buf)
		for _, r := range c.rows {
			if err := e.EncodeNext(r); err != nil {
				t.Errorf("EncodeNext(%v): %v", r, err)
			}
		}
		got := buf.String()
		if backcompat {
			got = strings.Replace(got, `""`, "", -1)
		}
		if got != c.want {
			t.Errorf("EncodeNext(%v): got %s, want %s", c.rows, got, c.want)
		}
	}
}

func TestEncode_Opts(t *testing.T) {
	rows := []struct{ A, B, C string }{
		{"a", "b", "c"},
		{"d", "e", "f"}}

	for _, c := range []struct {
		opts EncodeOpts
		want string
	}{{
		EncodeOpts{Comma: '%'},
		`A%B%C
a%b%c
d%e%f
`,
	}, {
		EncodeOpts{SkipHeader: true},
		`a,b,c
d,e,f
`,
	}, {
		EncodeOpts{UseCRLF: true},
		"A,B,C\r\na,b,c\r\nd,e,f\r\n",
	}} {
		var buf bytes.Buffer
		e := NewEncoder(&buf).Opts(c.opts)
		for _, r := range rows {
			if err := e.EncodeNext(r); err != nil {
				t.Errorf("EncodeNext(%v): %v", r, err)
			}
		}
		if got := buf.String(); got != c.want {
			t.Errorf("EncodeNext(%v): got %s, want %s", rows, got, c.want)
		}
	}
}

func TestEncode_Map(t *testing.T) {
	for _, c := range []struct {
		rows []map[string]interface{}
		want string
	}{{
		[]map[string]interface{}{{
			"foo": "a",
			"bar": true,
			"baz": 1.23,
			"ip":  ip,
		}, {
			"foo": "b",
			"bar": false,
			"baz": 4.56,
			"ip":  ip,
		}},
		// Keys are sorted before being written to the header
		`bar,baz,foo,ip
true,1.23,a,128.0.0.1
false,4.56,b,128.0.0.1
`,
	}, {
		[]map[string]interface{}{{
			"foo": "a",
		}, {
			"bar": "b",
		}},
		`foo
a
`,
	}, {
		[]map[string]interface{}{{
			"foo": "",
		}, {
			"foo": true,
		}},
		`foo

true
`,
	}} {
		var buf bytes.Buffer
		e := NewEncoder(&buf)
		for _, r := range c.rows {
			if err := e.EncodeNext(r); err != nil {
				t.Errorf("EncodeNext(%v): %v", r, err)
			}
		}
		got := buf.String()
		if backcompat {
			got = strings.Replace(got, `""`, "", -1)
		}
		if got != c.want {
			t.Errorf("EncodeNext(%v): got %s, want %s", c.rows, got, c.want)
		}
	}
}

// Tests that encoding a struct then encoding a compatible map works as expected.
func TestEncode_Hybrid(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	s := struct {
		Foo string `csv:"foo"`
		Bar string
	}{"a", "b"}
	if err := e.EncodeNext(s); err != nil {
		t.Errorf("EncodeNext(%v): %v", r, err)
	}
	m := map[string]interface{}{
		"foo": "c",
		"Bar": "d",
	}
	if err := e.EncodeNext(m); err != nil {
		t.Errorf("EncodeNext(%v): %v", r, err)
	}
	want := `foo,Bar
a,b
c,d
`
	if got := buf.String(); got != want {
		t.Errorf("EncodeNext(%v): got %s, want %s", m, got, want)
	}
}

// Tests that values that implement encoding.TextMarshaler are correctly marshaled.
func TestEncode_TextMarshaler(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	r := struct{ N net.IP }{ip}
	if err := e.EncodeNext(r); err != nil {
		t.Errorf("EncodeNext(%v): %v", r, err)
	}
	want := `N
128.0.0.1
`
	if got := buf.String(); got != want {
		t.Errorf("EncodeNext(%v): got %s, want %s", r, got, want)
	}
}

// Tests that structs with pointer fields are encoded correctly.
func TestEncode_Ptrs(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	bar := "bar"
	s := struct {
		S  string
		SP *string
	}{bar, &bar}
	if err := e.EncodeNext(s); err != nil {
		t.Errorf("EncodeNext(%v): %v", r, err)
	}
	want := `S,SP
bar,bar
`
	if got := buf.String(); got != want {
		t.Errorf("EncodeNext(%v): got %s, want %s", s, got, want)
	}
}
