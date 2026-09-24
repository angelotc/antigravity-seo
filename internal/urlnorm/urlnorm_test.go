package urlnorm

import (
	"net/url"
	"testing"
)

func TestSame(t *testing.T) {
	base, _ := url.Parse("https://example.com/blog/post")
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://EXAMPLE.com/blog/post", "https://example.com/blog/post/", true},
		{"https://example.com:443/blog/post", "https://example.com/blog/post", true},
		{"/blog/post", "https://example.com/blog/post", true},
		{"post", "https://example.com/blog/post", true},
		{"https://example.com/blog/post#top", "https://example.com/blog/post", true},
		{"https://example.com/blog/post?utm_source=x", "https://example.com/blog/post", true},
		{"https://example.com/?b=2&a=1", "https://example.com/?a=1&b=2", true},
		{"https://example.com/物件", "https://example.com/%E7%89%A9%E4%BB%B6", true},
		{"https://例え.jp/", "https://xn--r8jz45g.jp/", true},
		{"http://example.com/blog/post", "https://example.com/blog/post", false},
		{"https://example.com/About", "https://example.com/about", false},
		{"https://example.com/?page=2", "https://example.com/", false},
		{"https://www.example.com/", "https://example.com/", false},
	}
	for _, c := range cases {
		if got := Same(c.a, c.b, base); got != c.want {
			t.Errorf("Same(%q, %q) = %v, want %v (keys %q vs %q)", c.a, c.b, got, c.want, Key(c.a, base), Key(c.b, base))
		}
	}
}

func TestNormalizeKeepsQuery(t *testing.T) {
	u, err := Normalize("HTTPS://Example.COM:443?utm_source=x#f", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.String(); got != "https://example.com/?utm_source=x" {
		t.Errorf("got %q", got)
	}
}
