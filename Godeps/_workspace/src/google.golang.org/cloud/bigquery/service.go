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
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

// service provides an internal abstraction to isolate the generated
// BigQuery API; most of this package uses this interface instead.
// The single implementation, *bigqueryService, contains all the knowledge
// of the generated BigQuery API.
type service interface {
	insertJob(ctx context.Context, job *bq.Job, projectId string) (*Job, error)
	getJobType(ctx context.Context, projectId, jobID string) (jobType, error)
	jobStatus(ctx context.Context, projectId, jobID string) (*JobStatus, error)
	listTables(ctx context.Context, projectID, datasetID, pageToken string) ([]*Table, string, error)

	// readQuery reads data resulting from a query job. If the job is not
	// yet complete, an errIncompleteJob is returned. readQuery may be
	// called repeatedly to wait for results indefinitely.
	readQuery(ctx context.Context, conf *readQueryConf, pageToken string) (*readDataResult, error)

	readTabledata(ctx context.Context, conf *readTableConf, pageToken string) (*readDataResult, error)
}

type bigqueryService struct {
	s *bq.Service
}

func newBigqueryService(client *http.Client) (*bigqueryService, error) {
	s, err := bq.New(client)
	if err != nil {
		return nil, fmt.Errorf("constructing bigquery client: %v", err)
	}

	return &bigqueryService{s: s}, nil
}

// getPages calls the supplied getPage function repeatedly until there are no pages left to get.
// token is the token of the initial page to start from.  Use an empty string to start from the beginning.
func getPages(token string, getPage func(token string) (nextToken string, err error)) error {
	for {
		var err error
		token, err = getPage(token)
		if err != nil {
			return err
		}
		if token == "" {
			return nil
		}
	}
}

func (s *bigqueryService) insertJob(ctx context.Context, job *bq.Job, projectID string) (*Job, error) {
	// TODO(mcgreevy): use ctx
	res, err := s.s.Jobs.Insert(projectID, job).Do()
	if err != nil {
		return nil, err
	}
	return &Job{service: s, projectID: projectID, jobID: res.JobReference.JobId}, nil
}

type pagingConf struct {
	recordsPerRequest    int64
	setRecordsPerRequest bool

	startIndex uint64
}

type readTableConf struct {
	projectID, datasetID, tableID string
	paging                        pagingConf
}

type readDataResult struct {
	pageToken string
	rows      [][]Value
	totalRows uint64
	schema    Schema
}

type readQueryConf struct {
	projectID, jobID string
	paging           pagingConf
}

func (s *bigqueryService) readTabledata(ctx context.Context, conf *readTableConf, pageToken string) (*readDataResult, error) {
	req := s.s.Tabledata.List(conf.projectID, conf.datasetID, conf.tableID).
		PageToken(pageToken).
		StartIndex(conf.paging.startIndex)

	if conf.paging.setRecordsPerRequest {
		req = req.MaxResults(conf.paging.recordsPerRequest)
	}

	res, err := req.Do()
	if err != nil {
		return nil, err
	}

	result := &readDataResult{
		pageToken: res.PageToken,
		rows:      convertRows(res.Rows),
		totalRows: uint64(res.TotalRows),
	}
	return result, nil
}

var errIncompleteJob = errors.New("internal error: query results not available because job is not complete")

// getQueryResultsTimeout controls the maximum duration of a request to the
// BigQuery GetQueryResults endpoint.  Setting a long timeout here does not
// cause increased overall latency, as results are returned as soon as they are
// available.
const getQueryResultsTimeout = time.Minute

func (s *bigqueryService) readQuery(ctx context.Context, conf *readQueryConf, pageToken string) (*readDataResult, error) {
	req := s.s.Jobs.GetQueryResults(conf.projectID, conf.jobID).
		PageToken(pageToken).
		StartIndex(conf.paging.startIndex).
		TimeoutMs(getQueryResultsTimeout.Nanoseconds() / 1000)

	if conf.paging.setRecordsPerRequest {
		req = req.MaxResults(conf.paging.recordsPerRequest)
	}

	res, err := req.Do()
	if err != nil {
		return nil, err
	}

	if !res.JobComplete {
		return nil, errIncompleteJob
	}

	result := &readDataResult{
		pageToken: res.PageToken,
		rows:      convertRows(res.Rows),
		totalRows: res.TotalRows,
		schema:    convertTableSchema(res.Schema),
	}
	return result, nil
}

func convertRows(rows []*bq.TableRow) [][]Value {
	convertRow := func(r *bq.TableRow) []Value {
		var values []Value
		for _, cell := range r.F {
			values = append(values, cell.V)
		}
		return values
	}

	var rs [][]Value
	for _, r := range rows {
		rs = append(rs, convertRow(r))
	}
	return rs
}

type jobType int

const (
	copyJobType jobType = iota
	extractJobType
	loadJobType
	queryJobType
)

func (s *bigqueryService) getJobType(ctx context.Context, projectID, jobID string) (jobType, error) {
	// TODO(mcgreevy): use ctx
	res, err := s.s.Jobs.Get(projectID, jobID).
		Fields("configuration").
		Do()

	if err != nil {
		return 0, err
	}

	switch {
	case res.Configuration.Copy != nil:
		return copyJobType, nil
	case res.Configuration.Extract != nil:
		return extractJobType, nil
	case res.Configuration.Load != nil:
		return loadJobType, nil
	case res.Configuration.Query != nil:
		return queryJobType, nil
	default:
		return 0, errors.New("unknown job type")
	}
}

func (s *bigqueryService) jobStatus(ctx context.Context, projectID, jobID string) (*JobStatus, error) {
	// TODO(mcgreevy): use ctx
	res, err := s.s.Jobs.Get(projectID, jobID).
		Fields("status"). // Only fetch what we need.
		Do()
	if err != nil {
		return nil, err
	}
	return jobStatusFromProto(res.Status)
}

var stateMap = map[string]State{"PENDING": Pending, "RUNNING": Running, "DONE": Done}

func jobStatusFromProto(status *bq.JobStatus) (*JobStatus, error) {
	state, ok := stateMap[status.State]
	if !ok {
		return nil, fmt.Errorf("unexpected job state: %v", status.State)
	}

	newStatus := &JobStatus{
		State: state,
		err:   nil,
	}
	if err := errorFromErrorProto(status.ErrorResult); state == Done && err != nil {
		newStatus.err = err
	}

	for _, ep := range status.Errors {
		newStatus.Errors = append(newStatus.Errors, errorFromErrorProto(ep))
	}
	return newStatus, nil
}

// listTables returns a subset of tables that belong to a dataset, and a token for fetching the next subset.
func (s *bigqueryService) listTables(ctx context.Context, projectID, datasetID, pageToken string) ([]*Table, string, error) {
	// TODO(mcgreevy): use ctx
	var tables []*Table
	res, err := s.s.Tables.List(projectID, datasetID).
		PageToken(pageToken).
		Do()
	if err != nil {
		return nil, "", err
	}
	for _, t := range res.Tables {
		tables = append(tables, convertTable(t))
	}
	return tables, res.NextPageToken, nil
}

func convertTable(t *bq.TableListTables) *Table {
	return &Table{
		ProjectID: t.TableReference.ProjectId,
		DatasetID: t.TableReference.DatasetId,
		TableID:   t.TableReference.TableId,
	}
}
