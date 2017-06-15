/*
 *
 * Copyright 2017 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package status

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	apb "github.com/golang/protobuf/ptypes/any"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

func TestErrorsWithSameParameters(t *testing.T) {
	const description = "some description"
	e1 := Errorf(codes.AlreadyExists, description)
	e2 := Errorf(codes.AlreadyExists, description)
	if e1 == e2 || !reflect.DeepEqual(e1, e2) {
		t.Fatalf("Errors should be equivalent but unique - e1: %v, %v  e2: %p, %v", e1.(*statusError), e1, e2.(*statusError), e2)
	}
}

func TestFromToProto(t *testing.T) {
	s := &spb.Status{
		Code:    int32(codes.Internal),
		Message: "test test test",
		Details: []*apb.Any{{TypeUrl: "foo", Value: []byte{3, 2, 1}}},
	}

	err := FromProto(s)
	if got := err.Proto(); !proto.Equal(s, got) {
		t.Fatalf("Expected errors to be identical - s: %v  got: %v", s, got)
	}
}

func TestFromNilProto(t *testing.T) {
	tests := []*Status{nil, FromProto(nil)}
	for _, s := range tests {
		if c := s.Code(); c != codes.OK {
			t.Errorf("s: %v - Expected s.Code() = OK; got %v", s, c)
		}
		if m := s.Message(); m != "" {
			t.Errorf("s: %v - Expected s.Message() = \"\"; got %q", s, m)
		}
		if p := s.Proto(); p != nil {
			t.Errorf("s: %v - Expected s.Proto() = nil; got %q", s, p)
		}
		if e := s.Err(); e != nil {
			t.Errorf("s: %v - Expected s.Err() = nil; got %v", s, e)
		}
	}
}

func TestError(t *testing.T) {
	err := Error(codes.Internal, "test description")
	if got, want := err.Error(), "rpc error: code = Internal desc = test description"; got != want {
		t.Fatalf("err.Error() = %q; want %q", got, want)
	}
	s, _ := FromError(err)
	if got, want := s.Code(), codes.Internal; got != want {
		t.Fatalf("err.Code() = %s; want %s", got, want)
	}
	if got, want := s.Message(), "test description"; got != want {
		t.Fatalf("err.Message() = %s; want %s", got, want)
	}
}

func TestErrorOK(t *testing.T) {
	err := Error(codes.OK, "foo")
	if err != nil {
		t.Fatalf("Error(codes.OK, _) = %p; want nil", err.(*statusError))
	}
}

func TestErrorProtoOK(t *testing.T) {
	s := &spb.Status{Code: int32(codes.OK)}
	if got := ErrorProto(s); got != nil {
		t.Fatalf("ErrorProto(%v) = %v; want nil", s, got)
	}
}

func TestFromError(t *testing.T) {
	code, message := codes.Internal, "test description"
	err := Error(code, message)
	s, ok := FromError(err)
	if !ok || s.Code() != code || s.Message() != message || s.Err() == nil {
		t.Fatalf("FromError(%v) = %v, %v; want <Code()=%s, Message()=%q, Err()!=nil>, true", err, s, ok, code, message)
	}
}

func TestFromErrorOK(t *testing.T) {
	code, message := codes.OK, ""
	s, ok := FromError(nil)
	if !ok || s.Code() != code || s.Message() != message || s.Err() != nil {
		t.Fatalf("FromError(nil) = %v, %v; want <Code()=%s, Message()=%q, Err=nil>, true", s, ok, code, message)
	}
}
