package sandbox

import "testing"

func TestResolveWorkspacePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "/workspace"},
		{"tetris.html", "/workspace/tetris.html"},
		{"/tetris.html", "/workspace/tetris.html"},
		{"/workspace/tetris.html", "/workspace/tetris.html"},
		{"workspace/tetris.html", "/workspace/workspace/tetris.html"}, // relative segment
	}
	for _, c := range cases {
		got, err := ResolveWorkspacePath(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}
