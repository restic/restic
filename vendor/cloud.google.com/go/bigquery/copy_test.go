// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"testing"

	"cloud.google.com/go/internal/testutil"

	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

func defaultCopyJob() *bq.Job {
	return &bq.Job{
		JobReference: &bq.JobReference{ProjectId: "client-project-id"},
		Configuration: &bq.JobConfiguration{
			Copy: &bq.JobConfigurationTableCopy{
				DestinationTable: &bq.TableReference{
					ProjectId: "d-project-id",
					DatasetId: "d-dataset-id",
					TableId:   "d-table-id",
				},
				SourceTables: []*bq.TableReference{
					{
						ProjectId: "s-project-id",
						DatasetId: "s-dataset-id",
						TableId:   "s-table-id",
					},
				},
			},
		},
	}
}

func TestCopy(t *testing.T) {
	testCases := []struct {
		dst    *Table
		srcs   []*Table
		config CopyConfig
		want   *bq.Job
	}{
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			want: defaultCopyJob(),
		},
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			config: CopyConfig{
				CreateDisposition: CreateNever,
				WriteDisposition:  WriteTruncate,
			},
			want: func() *bq.Job {
				j := defaultCopyJob()
				j.Configuration.Copy.CreateDisposition = "CREATE_NEVER"
				j.Configuration.Copy.WriteDisposition = "WRITE_TRUNCATE"
				return j
			}(),
		},
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			config: CopyConfig{JobID: "job-id"},
			want: func() *bq.Job {
				j := defaultCopyJob()
				j.JobReference.JobId = "job-id"
				return j
			}(),
		},
	}

	for i, tc := range testCases {
		s := &testService{}
		c := &Client{
			service:   s,
			projectID: "client-project-id",
		}
		tc.dst.c = c
		copier := tc.dst.CopierFrom(tc.srcs...)
		tc.config.Srcs = tc.srcs
		tc.config.Dst = tc.dst
		copier.CopyConfig = tc.config
		if _, err := copier.Run(context.Background()); err != nil {
			t.Errorf("#%d: err calling Run: %v", i, err)
			continue
		}
		checkJob(t, i, s.Job, tc.want)
	}
}

func checkJob(t *testing.T, i int, got, want *bq.Job) {
	if got.JobReference == nil {
		t.Errorf("#%d: empty job  reference", i)
		return
	}
	if got.JobReference.JobId == "" {
		t.Errorf("#%d: empty job ID", i)
		return
	}
	d := testutil.Diff(got, want, cmpopts.IgnoreFields(bq.JobReference{}, "JobId"))
	if d != "" {
		t.Errorf("#%d: (got=-, want=+) %s", i, d)
	}
}
