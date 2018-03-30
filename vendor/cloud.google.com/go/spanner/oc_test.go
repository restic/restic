// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build go1.8

package spanner

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner/internal/testutil"
	"go.opencensus.io/plugin/ocgrpc"
	statsview "go.opencensus.io/stats/view"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func TestOCStats(t *testing.T) {
	// Check that stats are being exported.
	te := &testExporter{c: make(chan *statsview.Data)}
	statsview.RegisterExporter(te)
	defer statsview.UnregisterExporter(te)
	statsview.SetReportingPeriod(time.Millisecond)
	if err := ocgrpc.ClientRequestCountView.Subscribe(); err != nil {
		t.Fatal(err)
	}
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	ctx := context.Background()
	c, err := NewClient(ctx, "projects/P/instances/I/databases/D",
		option.WithEndpoint(ms.Addr()),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Single().ReadRow(ctx, "Users", Key{"alice"}, []string{"email"})
	// Wait until we see data from the view.
	select {
	case <-te.c:
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}

type testExporter struct {
	c chan *statsview.Data
}

func (e *testExporter) ExportView(vd *statsview.Data) {
	if len(vd.Rows) > 0 {
		select {
		case e.c <- vd:
		default:
		}
	}
}
