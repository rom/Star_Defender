package main

import "testing"

func TestListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		port int
		env  string
		want string
		bad  bool
	}{
		{"", 0, "", ":8080", false},
		{"", 9000, "", ":9000", false},
		{"", 0, "7000", ":7000", false},
		{"", 9000, "7000", ":9000", false},
		{"127.0.0.1:1234", 9000, "7000", "127.0.0.1:1234", false},
		{"", 70000, "", "", true},
		{"", -1, "", "", true},
		{"", 0, "abc", "", true},
	}
	for _, c := range cases {
		got, err := listenAddr(c.addr, c.port, c.env)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("listenAddr(%q, %d, %q) = %q, %v; want %q", c.addr, c.port, c.env, got, err, c.want)
		}
	}
}
