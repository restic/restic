package onedrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"golang.org/x/oauth2"
)

// Config contains all configuration necessary to connect to onedrive.
type Config struct {
	Token        oauth2.Token `json:"token"`
	ClientID     string       `json:"client_id"`
	ClientSecret string       `json:"client_secret"`
}

// ParseConfig parses the string s and extracts the onedrive config. The
// supported configuration format is onedrive:configfile.json
func ParseConfig(s string) (interface{}, error) {
	if strings.HasPrefix(s, "onedrive:") {
		s = s[9:]

		data := strings.SplitN(s, ":", 1)
		if len(data) != 1 {
			return nil, errors.New("onedrive: invalid format")
		}

		file, err := ioutil.ReadFile(data[0])
		if err != nil {
			return nil, fmt.Errorf("onedrive: reading tokenfile (%s) failed with '%s'", data[0], err)
		}

		var cfg Config
		if err := json.Unmarshal(file, &cfg); err != nil {
			return nil, fmt.Errorf("onedrive: token de-serialization failed with '%s'", err)
		}

		return cfg, nil
	}

	return nil, errors.New("onedrive: invalid format")
}
