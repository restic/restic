package azure

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "azure:container-name:/", Cfg: Config{
		Container:   "container-name",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "azure:container-name:/prefix/directory", Cfg: Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "azure:container-name:/prefix/directory/", Cfg: Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

func TestGetURL(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "Custom domain with HTTPS",
			config: Config{
				CustomDomain: "custom.com",
				Container:    "container",
			},
			expected: "https://custom.com/container",
		},
		{
			name: "Custom domain with HTTP",
			config: Config{
				CustomDomain: "custom.com",
				Container:    "container",
				UseHTTP:      true,
			},
			expected: "http://custom.com/container",
		},
		{
			name: "Default domain with custom endpoint suffix",
			config: Config{
				AccountName:    "account",
				Container:      "container",
				EndpointSuffix: "custom.net",
			},
			expected: "https://account.blob.custom.net/container",
		},
		{
			name: "Default domain with default endpoint suffix",
			config: Config{
				AccountName: "account",
				Container:   "container",
			},
			expected: "https://account.blob.core.windows.net/container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.getURL()
			if result != tt.expected {
				t.Errorf("got %s, want %s", result, tt.expected)
			}
		})
	}
}
