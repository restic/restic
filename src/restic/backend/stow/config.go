package stow

import (
	"net/url"
	"path"
	"strings"

	"restic/errors"
	"github.com/graymeta/stow"
	stows3 "github.com/graymeta/stow/s3"
	stowaz "github.com/graymeta/stow/azure"
	stowgs "github.com/graymeta/stow/google"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Kind      string
	ConfigMap stow.ConfigMap
	Bucket        string
	Prefix        string
}

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host:bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "azure://"):
		s = s[8:]
		return createAzureConfig(s)
	case strings.HasPrefix(s, "azure:"):
		s = s[6:]
		return createAzureConfig(s)
	case strings.HasPrefix(s, "gs://"):
		s = s[5:]
		return createGCSConfig(s)
	case strings.HasPrefix(s, "gs:"):
		s = s[3:]
		return createGCSConfig(s)
	case strings.HasPrefix(s, "aws:http"):
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s[4:])
		if err != nil {
			return nil, errors.Wrap(err, "url.Parse")
		}

		if url.Path == "" {
			return nil, errors.New("AWS s3: bucket name not found")
		}

		path := strings.SplitN(url.Path[1:], "/", 2)
		return createS3Config(url.Host, path, url.Scheme == "http")
	case strings.HasPrefix(s, "aws://"):
		s = s[6:]
	case strings.HasPrefix(s, "aws:"):
		s = s[4:]
	default:
		return nil, errors.New("s3: invalid format")
	}
	// use the first entry of the path as the endpoint and the
	// remainder as bucket name and prefix
	path := strings.SplitN(s, "/", 3)
	return createS3Config(path[0], path[1:], false)
}

func createAzureConfig(s string) (interface{}, error) {
	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	p := strings.SplitN(s, "/", 2)
	var prefix string
	switch {
	case len(p) < 1:
		return nil, errors.New("azure: invalid format, bucket name not found")
	case len(p) == 1 || p[1] == "":
		prefix = defaultPrefix
	default:
		prefix = path.Clean(p[1])
	}
	return Config{
		Kind:      stowaz.Kind,
		ConfigMap: stow.ConfigMap{},
		Bucket:    p[0],
		Prefix:    prefix,
	}, nil
}

func createGCSConfig(s string) (interface{}, error) {
	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	p := strings.SplitN(s, "/", 2)
	var prefix string
	switch {
	case len(p) < 1:
		return nil, errors.New("gs: invalid format, bucket name not found")
	case len(p) == 1 || p[1] == "":
		prefix = defaultPrefix
	default:
		prefix = path.Clean(p[1])
	}
	return Config{
		Kind:      stowgs.Kind,
		ConfigMap: stow.ConfigMap{},
		Bucket:    p[0],
		Prefix:    prefix,
	}, nil
}

func createS3Config(endpoint string, p []string, useHTTP bool) (interface{}, error) {
	var prefix string
	switch {
	case len(p) < 1:
		return nil, errors.New("AWS s3: invalid format, host/region or bucket name not found")
	case len(p) == 1 || p[1] == "":
		prefix = defaultPrefix
	default:
		prefix = path.Clean(p[1])
	}
	cfg := Config{
		Kind:      stows3.Kind,
		ConfigMap: stow.ConfigMap{},
		Bucket:   p[0],
		Prefix:   prefix,
	}
	cfg.ConfigMap[stows3.ConfigRegion] = getAWSRegion(endpoint)
	if useHTTP {
		cfg.ConfigMap[stows3.ConfigEndpoint] = endpoint
	}
	return cfg, nil
}

// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
// https://github.com/minio/cookbook/blob/master/docs/aws-sdk-for-go-with-minio.md
func getAWSRegion(endpoint string) string {
	var r string
	if endpoint == "s3.amazonaws.com" || endpoint == "s3-external-1.amazonaws.com" {
		return "us-east-1"
	} else if strings.HasPrefix(endpoint, "http://") {
		return "us-east-1" // minio
	} else if strings.HasPrefix(endpoint, "s3.dualstack.") {
		r = endpoint[len("s3.dualstack."):]
	} else {
		r = endpoint[3:] // s3- or s3.
	}
	return r[:strings.Index(r, ".")]
}
