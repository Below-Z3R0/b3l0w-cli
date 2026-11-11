package confirm

import "testing"

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		in        string
		def, want bool
		ok        bool
	}{
		{"", true, true, true},          // Enter con [Y/n]
		{"", false, false, true},        // Enter con [y/N]
		{"y", false, true, true},        // minúscula
		{"Y", false, true, true},        // mayúscula
		{" yes ", false, true, true},    // con espacios
		{"n", true, false, true},        // no
		{"N", true, false, true},        // no, mayúscula
		{"no", true, false, true},       // no, palabra
		{"s", true, false, false},       // español ya no se acepta
		{"tal vez", true, false, false}, // respuesta inválida
	}

	for _, c := range cases {
		got, ok := parseAnswer(c.in, c.def)
		if ok != c.ok {
			t.Errorf("parseAnswer(%q) ok = %v, quería %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("parseAnswer(%q) = %v, quería %v", c.in, got, c.want)
		}
	}
}
