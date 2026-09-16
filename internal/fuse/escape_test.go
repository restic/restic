package fuse

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	rtest "github.com/restic/restic/internal/test"
)

var escapeTests = []struct {
	name, escaped string
}{
	{"plain.txt", "plain.txt"},
	{".hidden", ".hidden"},
	{"a:b", "a：b"},
	{`back\slash`, "back＼slash"},
	{"star*q?", "star＊q？"},
	{`"quoted"<>|`, "＂quoted＂＜＞｜"},
	{"trailing.", "trailing．"},
	{"trailing ", "trailing␠"},
	{"dots..", "dots.．"},
	{" leading", " leading"},
	{"tab\there", "tab␉here"},
	{"nl\n", "nl␊"},
	{"nul", "‛nul"},
	{"CON.txt", "‛CON.txt"},
	{"com1", "‛com1"},
	{"LPT9.log", "‛LPT9.log"},
	{"COM¹", "‛COM¹"},
	{"console", "console"},
	{"com10", "com10"},
	{"nul.", "‛nul．"},
	{"already：wide", "already‛：wide"},
	{"quote‛here", "quote‛‛here"},
	{"‛nul", "‛‛nul"},
	{"pic␉ture", "pic‛␉ture"},
	{"", ""},
	{"日本語:名", "日本語：名"},
}

// isValidWindowsName reports whether name can be represented on a Windows file system.
func isValidWindowsName(name string) bool {
	if name == "" {
		return true
	}
	if strings.ContainsAny(name, `\:*?"<>|`) || isReservedWindowsName(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	last, _ := utf8.DecodeLastRuneInString(name)
	return last != '.' && last != ' '
}

func TestEscapeWindowsName(t *testing.T) {
	for _, test := range escapeTests {
		t.Run(test.name, func(t *testing.T) {
			escaped := escapeWindowsName(test.name)
			rtest.Equals(t, test.escaped, escaped)
			rtest.Assert(t, isValidWindowsName(escaped), "%q is not a valid Windows name", escaped)
			rtest.Equals(t, test.name, unescapeWindowsName(escaped))
		})
	}
}

func TestEscapeWindowsNameRoundTrip(t *testing.T) {
	alphabet := []rune("ab.: \\*?\"<>|\t\n‛：．␠␉␀xé日")
	rnd := rand.New(rand.NewSource(42))

	for range 2000 {
		runes := make([]rune, rnd.Intn(12))
		for i := range runes {
			runes[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		name := string(runes)

		escaped := escapeWindowsName(name)
		rtest.Assert(t, isValidWindowsName(escaped), "%q escaped to invalid name %q", name, escaped)
		rtest.Assert(t, unescapeWindowsName(escaped) == name, "round trip of %q failed: escaped %q, unescaped %q", name, escaped, unescapeWindowsName(escaped))
	}
}

func TestUnescapeWindowsNameLenient(t *testing.T) {
	// names that were not produced by escapeWindowsName must not panic and
	// should keep as much as possible
	rtest.Equals(t, "dangling‛", unescapeWindowsName("dangling‛"))
	rtest.Equals(t, "plain", unescapeWindowsName("plain"))
}
