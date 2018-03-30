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

package storage

import (
	"testing"

	"go.opencensus.io/trace"
	"golang.org/x/net/context"
)

func TestIntegration_OCTracing(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}

	te := &testExporter{}
	trace.RegisterExporter(te)
	defer trace.UnregisterExporter(te)
	trace.SetDefaultSampler(trace.AlwaysSample())

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucketName)
	bkt.Attrs(ctx)

	if len(te.spans) != 1 {
		t.Fatalf("Expected 1 span to be created, but got %d", len(te.spans))
	}
}

type testExporter struct {
	spans []*trace.SpanData
}

func (te *testExporter) ExportSpan(s *trace.SpanData) {
	te.spans = append(te.spans, s)
}
