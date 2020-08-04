package main

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/restic/restic/internal/restic"
)

func TestContentHash_MarshalText(t *testing.T) {
	table := []struct {
		name    string
		content []string
		want    string
	}{
		{
			name:    "empty",
			content: nil,
			want:    `""`,
		},
		{
			name: "single",
			content: []string{
				"77c2a7ef1cb99b134e64b33752834e76e76ae487a1fa7e82c044d9c82df7d304",
			},
			want: `"sha256:77c2a7ef1cb99b134e64b33752834e76e76ae487a1fa7e82c044d9c82df7d304"`,
		},
		{
			name: "multi",
			content: []string{
				"77c2a7ef1cb99b134e64b33752834e76e76ae487a1fa7e82c044d9c82df7d304",
				"8d02bb72e9a86e37a16219cfa43d996ce5751f4eee19c0e27b4b627ddf6b40fc",
			},
			want: `"multi:d252c57fe80290ad98c37e16b1728b70292f346098fe045fc805032033c23d63"`,
		},
	}

	for _, row := range table {
		t.Run(row.name, func(t *testing.T) {
			var ch contentHash
			for _, h := range row.content {
				testHash, err := hex.DecodeString(h)
				if err != nil {
					t.Fatalf("hex decode error: %v", err)
				}
				testID := restic.IDFromHash(testHash)
				ch = append(ch, testID)
			}
			// We test MarshalText indirectly through the json modules, because
			// that is what we are actually interested in.
			jsonBytes, err := json.Marshal(ch)
			if err != nil {
				t.Fatalf("json marshal error: %v", err)
			}
			jsonString := string(jsonBytes)
			if jsonString != row.want {
				t.Fatalf("error: got %s, want %s", jsonString, row.want)
			}
		})
	}
}
