package update

import "testing"

func TestConfirmPrompt(t *testing.T) {
	cases := []struct {
		self, system, programs, plugins, services, dotfiles bool
		want                                                string
	}{
		{true, true, true, true, true, true, "Update b3l0w, system, programs, plugins, services, dotfiles? [Y/n] "},
		{false, true, false, false, false, false, "Update system? [Y/n] "},
		{false, false, true, false, false, false, "Update programs? [Y/n] "},
		{false, false, false, true, false, false, "Update plugins? [Y/n] "},
		{false, false, false, false, true, false, "Update services? [Y/n] "},
		{false, false, false, false, false, true, "Update dotfiles? [Y/n] "},
		{true, false, false, false, false, false, "Update b3l0w? [Y/n] "},
	}
	for _, c := range cases {
		got := confirmPrompt(c.self, c.system, c.programs, c.plugins, c.services, c.dotfiles)
		if got != c.want {
			t.Errorf("confirmPrompt(%v,%v,%v,%v,%v,%v) = %q, quería %q",
				c.self, c.system, c.programs, c.plugins, c.services, c.dotfiles, got, c.want)
		}
	}
}
