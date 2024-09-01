package restic

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

func parseTimeNano(t testing.TB, s string) time.Time {
	// 2006-01-02T15:04:05.999999999Z07:00
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("error parsing %q: %v", s, err)
	}
	return ts
}

func TestFixTime(t *testing.T) {
	// load UTC location
	utc, err := time.LoadLocation("")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		src, want time.Time
	}{
		{
			src:  parseTimeNano(t, "2006-01-02T15:04:05.999999999+07:00"),
			want: parseTimeNano(t, "2006-01-02T15:04:05.999999999+07:00"),
		},
		{
			src:  time.Date(0, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "0000-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(-2, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "0000-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(12345, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "9999-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(9999, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "9999-01-02T03:04:05.000000006+00:00"),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res := FixTime(test.src)
			if !res.Equal(test.want) {
				t.Fatalf("wrong result for %v, want:\n  %v\ngot:\n  %v", test.src, test.want, res)
			}
		})
	}
}

func TestSymlinkSerialization(t *testing.T) {
	for _, link := range []string{
		"válîd \t Üñi¢òde \n śẗŕinǵ",
		string([]byte{0, 1, 2, 0xfa, 0xfb, 0xfc}),
	} {
		n := Node{
			LinkTarget: link,
		}
		ser, err := json.Marshal(n)
		test.OK(t, err)
		var n2 Node
		err = json.Unmarshal(ser, &n2)
		test.OK(t, err)
		fmt.Println(string(ser))

		test.Equals(t, n.LinkTarget, n2.LinkTarget)
	}
}

func TestSymlinkSerializationFormat(t *testing.T) {
	for _, d := range []struct {
		ser        string
		linkTarget string
	}{
		{`{"linktarget":"test"}`, "test"},
		{`{"linktarget":"\u0000\u0001\u0002\ufffd\ufffd\ufffd","linktarget_raw":"AAEC+vv8"}`, string([]byte{0, 1, 2, 0xfa, 0xfb, 0xfc})},
	} {
		var n2 Node
		err := json.Unmarshal([]byte(d.ser), &n2)
		test.OK(t, err)
		test.Equals(t, d.linkTarget, n2.LinkTarget)
		test.Assert(t, n2.LinkTargetRaw == nil, "quoted link target is just a helper field and must be unset after decoding")
	}
}
