package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kurin/blazer/bonfire"
	"github.com/kurin/blazer/internal/pyre"
)

type superManager struct {
	*bonfire.LocalBucket
	bonfire.FS
}

func main() {
	ctx := context.Background()
	mux := http.NewServeMux()

	fs := bonfire.FS("/tmp/b2")
	bm := &bonfire.LocalBucket{Port: 8822}

	if err := pyre.RegisterServerOnMux(ctx, &pyre.Server{
		Account:   bonfire.Localhost(8822),
		LargeFile: fs,
		Bucket:    bm,
	}, mux); err != nil {
		fmt.Println(err)
		return
	}

	sm := superManager{
		LocalBucket: bm,
		FS:          fs,
	}

	pyre.RegisterLargeFileManagerOnMux(fs, mux)
	pyre.RegisterSimpleFileManagerOnMux(fs, mux)
	pyre.RegisterDownloadManagerOnMux(sm, mux)
	fmt.Println("ok")
	fmt.Println(http.ListenAndServe("localhost:8822", mux))
}
