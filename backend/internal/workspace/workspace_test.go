package workspace

import "testing"

func TestSplitHostPath(t *testing.T) {
	cases := []struct{ in, anchor, rest string }{
		{`C:\Users\me\app`, `C:\`, "Users/me/app"},
		{`d:/code/app/`, `d:\`, "code/app"},
		{`C:\`, `C:\`, ""},
		{"/Users/me/app", "/Users", "me/app"},
		{"/home/me/app/", "/home", "me/app"},
		{"/Volumes", "/Volumes", ""},
	}
	for _, c := range cases {
		anchor, rest, err := splitHostPath(c.in)
		if err != nil || anchor != c.anchor || rest != c.rest {
			t.Errorf("splitHostPath(%q) = %q, %q, %v; want %q, %q", c.in, anchor, rest, err, c.anchor, c.rest)
		}
	}
	for _, bad := range []string{"projects/app", "/", "", `\server\share`} {
		if _, _, err := splitHostPath(bad); err == nil {
			t.Errorf("splitHostPath(%q) should fail", bad)
		}
	}
}
