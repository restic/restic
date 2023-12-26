package rest_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/rest"
)

func TestListAPI(t *testing.T) {
	var tests = []struct {
		Name string

		ContentType string // response header
		Data        string // response data
		Requests    int

		Result []backend.FileInfo
	}{
		{
			Name:        "content-type-unknown",
			ContentType: "application/octet-stream",
			Data: `[
				"1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985",
				"3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352",
				"8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b"
			]`,
			Result: []backend.FileInfo{
				{Name: "1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985", Size: 4386},
				{Name: "3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352", Size: 15214},
				{Name: "8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b", Size: 33393},
			},
			Requests: 4,
		},
		{
			Name:        "content-type-v1",
			ContentType: "application/vnd.x.restic.rest.v1",
			Data: `[
				"1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985",
				"3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352",
				"8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b"
			]`,
			Result: []backend.FileInfo{
				{Name: "1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985", Size: 4386},
				{Name: "3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352", Size: 15214},
				{Name: "8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b", Size: 33393},
			},
			Requests: 4,
		},
		{
			Name:        "content-type-v2",
			ContentType: "application/vnd.x.restic.rest.v2",
			Data: `[
				{"name": "1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985", "size": 1001},
				{"name": "3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352", "size": 1002},
				{"name": "8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b", "size": 1003}
			]`,
			Result: []backend.FileInfo{
				{Name: "1122e6749358b057fa1ac6b580a0fbe7a9a5fbc92e82743ee21aaf829624a985", Size: 1001},
				{Name: "3b6ec1af8d4f7099d0445b12fdb75b166ba19f789e5c48350c423dc3b3e68352", Size: 1002},
				{Name: "8271d221a60e0058e6c624f248d0080fc04f4fac07a28584a9b89d0eb69e189b", Size: 1003},
			},
			Requests: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			numRequests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				numRequests++
				t.Logf("req %v %v, accept: %v", req.Method, req.URL.Path, req.Header["Accept"])

				var err error
				switch {
				case req.Method == "GET":
					// list files in data/
					res.Header().Set("Content-Type", test.ContentType)
					_, err = res.Write([]byte(test.Data))

					if err != nil {
						t.Fatal(err)
					}
					return
				case req.Method == "HEAD":
					// stat file in data/, use the first two bytes in the name
					// of the file as the size :)
					filename := req.URL.Path[6:]
					length, err := strconv.ParseInt(filename[:4], 16, 64)
					if err != nil {
						t.Fatal(err)
					}

					res.Header().Set("Content-Length", fmt.Sprintf("%d", length))
					res.WriteHeader(http.StatusOK)
					return
				}

				t.Errorf("unhandled request %v %v", req.Method, req.URL.Path)
			}))
			defer srv.Close()

			srvURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}

			cfg := rest.Config{
				Connections: 5,
				URL:         srvURL,
			}

			be, err := rest.Open(context.TODO(), cfg, http.DefaultTransport)
			if err != nil {
				t.Fatal(err)
			}

			var list []backend.FileInfo
			err = be.List(context.TODO(), backend.PackFile, func(fi backend.FileInfo) error {
				list = append(list, fi)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(list, test.Result) {
				t.Fatalf("wrong response returned, want:\n  %v\ngot:  %v", test.Result, list)
			}

			if numRequests != test.Requests {
				t.Fatalf("wrong number of HTTP requests executed, want %d, got %d", test.Requests, numRequests)
			}

			defer func() {
				err = be.Close()
				if err != nil {
					t.Fatal(err)
				}
			}()
		})
	}
}
