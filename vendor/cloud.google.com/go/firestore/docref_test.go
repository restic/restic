// Copyright 2017 Google Inc. All Rights Reserved.
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

package firestore

import (
	"reflect"
	"sort"
	"testing"
	"time"

	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	writeResultForSet    = &WriteResult{UpdateTime: aTime}
	commitResponseForSet = &pb.CommitResponse{
		WriteResults: []*pb.WriteResult{{UpdateTime: aTimestamp}},
	}
)

func TestDocGet(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	path := "projects/projectID/databases/(default)/documents/C/a"
	pdoc := &pb.Document{
		Name:       path,
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp,
		Fields:     map[string]*pb.Value{"f": intval(1)},
	}
	srv.addRPC(&pb.GetDocumentRequest{Name: path}, pdoc)
	ref := c.Collection("C").Doc("a")
	gotDoc, err := ref.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDoc := &DocumentSnapshot{
		Ref:        ref,
		CreateTime: aTime,
		UpdateTime: aTime,
		proto:      pdoc,
		c:          c,
	}
	if !testEqual(gotDoc, wantDoc) {
		t.Fatalf("\ngot  %+v\nwant %+v", gotDoc, wantDoc)
	}

	srv.addRPC(
		&pb.GetDocumentRequest{
			Name: "projects/projectID/databases/(default)/documents/C/b",
		},
		grpc.Errorf(codes.NotFound, "not found"),
	)
	_, err = c.Collection("C").Doc("b").Get(ctx)
	if grpc.Code(err) != codes.NotFound {
		t.Errorf("got %v, want NotFound", err)
	}
}

func TestDocSet(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	for _, test := range []struct {
		desc      string
		data      interface{}
		opt       SetOption
		write     map[string]*pb.Value
		mask      []string
		transform []string
		isErr     bool
	}{
		{
			desc:  "Set with no options",
			data:  map[string]interface{}{"a": 1},
			write: map[string]*pb.Value{"a": intval(1)},
		},
		{
			desc:  "Merge with a field",
			data:  map[string]interface{}{"a": 1, "b": 2},
			opt:   Merge("a"),
			write: map[string]*pb.Value{"a": intval(1)},
			mask:  []string{"a"},
		},
		{
			desc: "Merge field is not a leaf",
			data: map[string]interface{}{
				"a": map[string]interface{}{"b": 1, "c": 2},
				"d": 3,
			},
			opt: Merge("a"),
			write: map[string]*pb.Value{"a": mapval(map[string]*pb.Value{
				"b": intval(1),
				"c": intval(2),
			})},
			mask: []string{"a"},
		},
		{
			desc:  "MergeAll",
			data:  map[string]interface{}{"a": 1, "b": 2},
			opt:   MergeAll,
			write: map[string]*pb.Value{"a": intval(1), "b": intval(2)},
			mask:  []string{"a", "b"},
		},
		{
			desc: "MergeAll with nested fields",
			data: map[string]interface{}{
				"a": 1,
				"b": map[string]interface{}{"c": 2},
			},
			opt: MergeAll,
			write: map[string]*pb.Value{
				"a": intval(1),
				"b": mapval(map[string]*pb.Value{"c": intval(2)}),
			},
			mask: []string{"a", "b.c"},
		},
		{
			desc: "Merge with FieldPaths",
			data: map[string]interface{}{"*": map[string]interface{}{"~": true}},
			opt:  MergePaths([]string{"*", "~"}),
			write: map[string]*pb.Value{
				"*": mapval(map[string]*pb.Value{
					"~": boolval(true),
				}),
			},
			mask: []string{"`*`.`~`"},
		},
		{
			desc: "Merge with a struct and FieldPaths",
			data: struct {
				A map[string]bool `firestore:"*"`
			}{A: map[string]bool{"~": true}},
			opt: MergePaths([]string{"*", "~"}),
			write: map[string]*pb.Value{
				"*": mapval(map[string]*pb.Value{
					"~": boolval(true),
				}),
			},
			mask: []string{"`*`.`~`"},
		},
		{
			desc:      "a ServerTimestamp field becomes a transform",
			data:      map[string]interface{}{"a": 1, "b": ServerTimestamp},
			write:     map[string]*pb.Value{"a": intval(1)},
			transform: []string{"b"},
		},
		{
			desc:      "a ServerTimestamp alone",
			data:      map[string]interface{}{"b": ServerTimestamp},
			write:     nil,
			transform: []string{"b"},
		},
		{
			desc:      "a ServerTimestamp alone with a path",
			data:      map[string]interface{}{"b": ServerTimestamp},
			opt:       MergePaths([]string{"b"}),
			write:     nil,
			transform: []string{"b"},
		},
		{
			desc: "nested ServerTimestamp field",
			data: map[string]interface{}{
				"a": 1,
				"b": map[string]interface{}{"c": ServerTimestamp},
			},
			write:     map[string]*pb.Value{"a": intval(1)},
			transform: []string{"b.c"},
		},
		{
			desc: "multiple ServerTimestamp fields",
			data: map[string]interface{}{
				"a": 1,
				"b": ServerTimestamp,
				"c": map[string]interface{}{"d": ServerTimestamp},
			},
			write:     map[string]*pb.Value{"a": intval(1)},
			transform: []string{"b", "c.d"},
		},
		{
			desc:      "ServerTimestamp with MergeAll",
			data:      map[string]interface{}{"a": 1, "b": ServerTimestamp},
			opt:       MergeAll,
			write:     map[string]*pb.Value{"a": intval(1)},
			mask:      []string{"a"},
			transform: []string{"b"},
		},
		{
			desc:      "ServerTimestamp with Merge of both fields",
			data:      map[string]interface{}{"a": 1, "b": ServerTimestamp},
			opt:       Merge("a", "b"),
			write:     map[string]*pb.Value{"a": intval(1)},
			mask:      []string{"a"},
			transform: []string{"b"},
		},
		{
			desc:  "If is ServerTimestamp not in Merge, no transform",
			data:  map[string]interface{}{"a": 1, "b": ServerTimestamp},
			opt:   Merge("a"),
			write: map[string]*pb.Value{"a": intval(1)},
			mask:  []string{"a"},
		},
		{
			desc:      "If no ordinary values in Merge, no write",
			data:      map[string]interface{}{"a": 1, "b": ServerTimestamp},
			opt:       Merge("b"),
			transform: []string{"b"},
		},
		{
			desc:  "Merge fields must all be present in data.",
			data:  map[string]interface{}{"a": 1},
			opt:   Merge("b", "a"),
			isErr: true,
		},
		{
			desc:  "MergeAll cannot be used with structs",
			data:  struct{ A int }{A: 1},
			opt:   MergeAll,
			isErr: true,
		},
		{
			desc:  "Delete cannot appear in data",
			data:  map[string]interface{}{"a": 1, "b": Delete},
			isErr: true,
		},
		{
			desc:  "Delete cannot even appear in an unmerged field (allow?)",
			data:  map[string]interface{}{"a": 1, "b": Delete},
			opt:   Merge("a"),
			isErr: true,
		},
	} {
		srv.reset()
		if !test.isErr {
			var writes []*pb.Write
			if test.write != nil || test.mask != nil {
				w := &pb.Write{}
				if test.write != nil {
					w.Operation = &pb.Write_Update{
						Update: &pb.Document{
							Name:   "projects/projectID/databases/(default)/documents/C/d",
							Fields: test.write,
						},
					}
				}
				if test.mask != nil {
					w.UpdateMask = &pb.DocumentMask{FieldPaths: test.mask}
				}
				writes = append(writes, w)
			}
			if test.transform != nil {
				var fts []*pb.DocumentTransform_FieldTransform
				for _, p := range test.transform {
					fts = append(fts, &pb.DocumentTransform_FieldTransform{
						FieldPath:     p,
						TransformType: requestTimeTransform,
					})
				}
				writes = append(writes, &pb.Write{
					Operation: &pb.Write_Transform{
						&pb.DocumentTransform{
							Document:        "projects/projectID/databases/(default)/documents/C/d",
							FieldTransforms: fts,
						},
					},
				})
			}

			srv.addRPC(&pb.CommitRequest{
				Database: "projects/projectID/databases/(default)",
				Writes:   writes,
			}, commitResponseForSet)
		}
		var opts []SetOption
		if test.opt != nil {
			opts = []SetOption{test.opt}
		}
		wr, err := c.Collection("C").Doc("d").Set(ctx, test.data, opts...)
		if test.isErr && err == nil {
			t.Errorf("%s: got nil, want error")
			continue
		}
		if !test.isErr && err != nil {
			t.Errorf("%s: %v", test.desc, err)
			continue
		}
		if err == nil && !testEqual(wr, writeResultForSet) {
			t.Errorf("%s: got %v, want %v", test.desc, wr, writeResultForSet)
		}
	}
}

func TestDocCreate(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	wantReq := commitRequestForSet()
	wantReq.Writes[0].CurrentDocument = &pb.Precondition{
		ConditionType: &pb.Precondition_Exists{false},
	}
	srv.addRPC(wantReq, commitResponseForSet)
	wr, err := c.Collection("C").Doc("d").Create(ctx, testData)
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, writeResultForSet) {
		t.Errorf("got %v, want %v", wr, writeResultForSet)
	}

	// Verify creation with structs. In particular, make sure zero values
	// are handled well.
	type create struct {
		Time  time.Time
		Bytes []byte
		Geo   *latlng.LatLng
	}
	srv.addRPC(
		&pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name: "projects/projectID/databases/(default)/documents/C/d",
							Fields: map[string]*pb.Value{
								"Time":  tsval(time.Time{}),
								"Bytes": bytesval(nil),
								"Geo":   nullValue,
							},
						},
					},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{false},
					},
				},
			},
		},
		commitResponseForSet,
	)
	_, err = c.Collection("C").Doc("d").Create(ctx, &create{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDocDelete(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	srv.addRPC(
		&pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{
				{Operation: &pb.Write_Delete{"projects/projectID/databases/(default)/documents/C/d"}},
			},
		},
		&pb.CommitResponse{
			WriteResults: []*pb.WriteResult{{}},
		})
	wr, err := c.Collection("C").Doc("d").Delete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, &WriteResult{}) {
		t.Errorf("got %+v, want %+v", wr, writeResultForSet)
	}
}

func TestDocDeleteLastUpdateTime(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	wantReq := &pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{
			{
				Operation: &pb.Write_Delete{"projects/projectID/databases/(default)/documents/C/d"},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_UpdateTime{aTimestamp2},
				},
			}},
	}
	srv.addRPC(wantReq, commitResponseForSet)
	wr, err := c.Collection("C").Doc("d").Delete(ctx, LastUpdateTime(aTime2))
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, writeResultForSet) {
		t.Errorf("got %+v, want %+v", wr, writeResultForSet)
	}
}

var (
	testData   = map[string]interface{}{"a": 1}
	testFields = map[string]*pb.Value{"a": intval(1)}
)

func TestUpdateMap(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	for _, test := range []struct {
		data       map[string]interface{}
		wantFields map[string]*pb.Value
		wantPaths  []string
	}{
		{
			data: map[string]interface{}{"a.b": 1},
			wantFields: map[string]*pb.Value{
				"a": mapval(map[string]*pb.Value{"b": intval(1)}),
			},
			wantPaths: []string{"a.b"},
		},
		{
			data: map[string]interface{}{
				"a": 1,
				"b": Delete,
			},
			wantFields: map[string]*pb.Value{"a": intval(1)},
			wantPaths:  []string{"a", "b"},
		},
	} {
		srv.reset()
		wantReq := &pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{{
				Operation: &pb.Write_Update{
					Update: &pb.Document{
						Name:   "projects/projectID/databases/(default)/documents/C/d",
						Fields: test.wantFields,
					}},
				UpdateMask: &pb.DocumentMask{FieldPaths: test.wantPaths},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{true},
				},
			}},
		}
		// Sort update masks, because map iteration order is random.
		sort.Strings(wantReq.Writes[0].UpdateMask.FieldPaths)
		srv.addRPCAdjust(wantReq, commitResponseForSet, func(gotReq proto.Message) {
			sort.Strings(gotReq.(*pb.CommitRequest).Writes[0].UpdateMask.FieldPaths)
		})
		wr, err := c.Collection("C").Doc("d").UpdateMap(ctx, test.data)
		if err != nil {
			t.Fatal(err)
		}
		if !testEqual(wr, writeResultForSet) {
			t.Errorf("%v:\ngot %+v, want %+v", test.data, wr, writeResultForSet)
		}
	}
}

func TestUpdateMapLastUpdateTime(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)

	wantReq := &pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{{
			Operation: &pb.Write_Update{
				Update: &pb.Document{
					Name:   "projects/projectID/databases/(default)/documents/C/d",
					Fields: map[string]*pb.Value{"a": intval(1)},
				}},
			UpdateMask: &pb.DocumentMask{FieldPaths: []string{"a"}},
			CurrentDocument: &pb.Precondition{
				ConditionType: &pb.Precondition_UpdateTime{aTimestamp2},
			},
		}},
	}
	srv.addRPC(wantReq, commitResponseForSet)
	wr, err := c.Collection("C").Doc("d").UpdateMap(ctx, map[string]interface{}{"a": 1}, LastUpdateTime(aTime2))
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, writeResultForSet) {
		t.Errorf("got %v, want %v", wr, writeResultForSet)
	}
}

func TestUpdateMapErrors(t *testing.T) {
	ctx := context.Background()
	c, _ := newMock(t)
	for _, in := range []map[string]interface{}{
		nil, // no paths
		map[string]interface{}{"a~b": 1},         // invalid character
		map[string]interface{}{"a..b": 1},        // empty path component
		map[string]interface{}{"a.b": 1, "a": 2}, // prefix
	} {
		_, err := c.Collection("C").Doc("d").UpdateMap(ctx, in)
		if err == nil {
			t.Errorf("%v: got nil, want error", in)
		}
	}
}

func TestUpdateStruct(t *testing.T) {
	type update struct{ A int }
	c, srv := newMock(t)
	wantReq := &pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{{
			Operation: &pb.Write_Update{
				Update: &pb.Document{
					Name:   "projects/projectID/databases/(default)/documents/C/d",
					Fields: map[string]*pb.Value{"A": intval(2)},
				},
			},
			UpdateMask: &pb.DocumentMask{FieldPaths: []string{"A", "b.c"}},
			CurrentDocument: &pb.Precondition{
				ConditionType: &pb.Precondition_Exists{true},
			},
		}},
	}
	srv.addRPC(wantReq, commitResponseForSet)
	wr, err := c.Collection("C").Doc("d").
		UpdateStruct(context.Background(), []string{"A", "b.c"}, &update{A: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, writeResultForSet) {
		t.Errorf("got %+v, want %+v", wr, writeResultForSet)
	}
}

func TestUpdateStructErrors(t *testing.T) {
	type update struct{ A int }

	ctx := context.Background()
	c, _ := newMock(t)
	doc := c.Collection("C").Doc("d")
	for _, test := range []struct {
		desc   string
		fields []string
		data   interface{}
	}{
		{
			desc: "data is not a struct or *struct",
			data: map[string]interface{}{"a": 1},
		},
		{
			desc:   "no paths",
			fields: nil,
			data:   update{},
		},
		{
			desc:   "empty",
			fields: []string{""},
			data:   update{},
		},
		{
			desc:   "empty component",
			fields: []string{"a.b..c"},
			data:   update{},
		},
		{
			desc:   "duplicate field",
			fields: []string{"a", "b", "c", "a"},
			data:   update{},
		},
		{
			desc:   "invalid character",
			fields: []string{"a", "b]"},
			data:   update{},
		},
		{
			desc:   "prefix",
			fields: []string{"a", "b", "c", "b.c"},
			data:   update{},
		},
	} {
		_, err := doc.UpdateStruct(ctx, test.fields, test.data)
		if err == nil {
			t.Errorf("%s: got nil, want error", test.desc)
		}
	}
}

func TestUpdatePaths(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	for _, test := range []struct {
		data       []FieldPathUpdate
		wantFields map[string]*pb.Value
		wantPaths  []string
	}{
		{
			data: []FieldPathUpdate{
				{Path: []string{"*", "~"}, Value: 1},
				{Path: []string{"*", "/"}, Value: 2},
			},
			wantFields: map[string]*pb.Value{
				"*": mapval(map[string]*pb.Value{
					"~": intval(1),
					"/": intval(2),
				}),
			},
			wantPaths: []string{"`*`.`~`", "`*`.`/`"},
		},
		{
			data: []FieldPathUpdate{
				{Path: []string{"*"}, Value: 1},
				{Path: []string{"]"}, Value: Delete},
			},
			wantFields: map[string]*pb.Value{"*": intval(1)},
			wantPaths:  []string{"`*`", "`]`"},
		},
	} {
		srv.reset()
		wantReq := &pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{{
				Operation: &pb.Write_Update{
					Update: &pb.Document{
						Name:   "projects/projectID/databases/(default)/documents/C/d",
						Fields: test.wantFields,
					}},
				UpdateMask: &pb.DocumentMask{FieldPaths: test.wantPaths},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{true},
				},
			}},
		}
		// Sort update masks, because map iteration order is random.
		sort.Strings(wantReq.Writes[0].UpdateMask.FieldPaths)
		srv.addRPCAdjust(wantReq, commitResponseForSet, func(gotReq proto.Message) {
			sort.Strings(gotReq.(*pb.CommitRequest).Writes[0].UpdateMask.FieldPaths)
		})
		wr, err := c.Collection("C").Doc("d").UpdatePaths(ctx, test.data)
		if err != nil {
			t.Fatal(err)
		}
		if !testEqual(wr, writeResultForSet) {
			t.Errorf("%v:\ngot %+v, want %+v", test.data, wr, writeResultForSet)
		}
	}
}

func TestUpdatePathsErrors(t *testing.T) {
	fpu := func(s ...string) FieldPathUpdate { return FieldPathUpdate{Path: s} }

	ctx := context.Background()
	c, _ := newMock(t)
	doc := c.Collection("C").Doc("d")
	for _, test := range []struct {
		desc string
		data []FieldPathUpdate
	}{
		{"no updates", nil},
		{"empty", []FieldPathUpdate{fpu("")}},
		{"empty component", []FieldPathUpdate{fpu("*", "")}},
		{"duplicate field", []FieldPathUpdate{fpu("~"), fpu("*"), fpu("~")}},
		{"prefix", []FieldPathUpdate{fpu("*", "a"), fpu("b"), fpu("*", "a", "b")}},
	} {
		_, err := doc.UpdatePaths(ctx, test.data)
		if err == nil {
			t.Errorf("%s: got nil, want error", test.desc)
		}
	}
}

func TestApplyFieldPaths(t *testing.T) {
	submap := mapval(map[string]*pb.Value{
		"b": intval(1),
		"c": intval(2),
	})
	fields := map[string]*pb.Value{
		"a": submap,
		"d": intval(3),
	}
	for _, test := range []struct {
		fps  []FieldPath
		want map[string]*pb.Value
	}{
		{nil, nil},
		{[]FieldPath{[]string{"z"}}, nil},
		{[]FieldPath{[]string{"a"}}, map[string]*pb.Value{"a": submap}},
		{[]FieldPath{[]string{"a", "b", "c"}}, nil},
		{[]FieldPath{[]string{"d"}}, map[string]*pb.Value{"d": intval(3)}},
		{
			[]FieldPath{[]string{"d"}, []string{"a", "c"}},
			map[string]*pb.Value{
				"a": mapval(map[string]*pb.Value{"c": intval(2)}),
				"d": intval(3),
			},
		},
	} {
		got := applyFieldPaths(fields, test.fps, nil)
		if !testEqual(got, test.want) {
			t.Errorf("%v:\ngot %v\nwant \n%v", test.fps, got, test.want)
		}
	}
}

func TestFieldPathsFromMap(t *testing.T) {
	for _, test := range []struct {
		in   map[string]interface{}
		want []string
	}{
		{nil, nil},
		{map[string]interface{}{"a": 1}, []string{"a"}},
		{map[string]interface{}{
			"a": 1,
			"b": map[string]interface{}{"c": 2},
		}, []string{"a", "b.c"}},
	} {
		fps := fieldPathsFromMap(reflect.ValueOf(test.in), nil)
		got := toServiceFieldPaths(fps)
		sort.Strings(got)
		if !testEqual(got, test.want) {
			t.Errorf("%+v: got %v, want %v", test.in, got, test.want)
		}
	}
}

func commitRequestForSet() *pb.CommitRequest {
	return &pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{
			{
				Operation: &pb.Write_Update{
					Update: &pb.Document{
						Name:   "projects/projectID/databases/(default)/documents/C/d",
						Fields: testFields,
					},
				},
			},
		},
	}
}
