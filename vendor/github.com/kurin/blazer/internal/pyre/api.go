// Copyright 2018, Google
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

package pyre

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/kurin/blazer/internal/pyre/proto"
)

type apiErr struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func serveMuxOptions() []runtime.ServeMuxOption {
	return []runtime.ServeMuxOption{
		runtime.WithMarshalerOption("*", &runtime.JSONPb{}),
		runtime.WithProtoErrorHandler(func(ctx context.Context, mux *runtime.ServeMux, m runtime.Marshaler, rw http.ResponseWriter, req *http.Request, err error) {
			aErr := apiErr{
				Status:  400,
				Code:    "uh oh",
				Message: err.Error(),
			}
			rw.WriteHeader(aErr.Status)
			if err := m.NewEncoder(rw).Encode(aErr); err != nil {
				fmt.Fprintln(os.Stdout, err)
			}
		}),
	}
}

func getAuth(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}
	data := md.Get("authorization")
	if len(data) == 0 {
		return "", nil
	}
	return data[0], nil
}

func RegisterServerOnMux(ctx context.Context, srv *Server, mux *http.ServeMux) error {
	rmux := runtime.NewServeMux(serveMuxOptions()...)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return err
	}
	gsrv := grpc.NewServer()
	if err := pb.RegisterPyreServiceHandlerFromEndpoint(ctx, rmux, l.Addr().String(), []grpc.DialOption{grpc.WithInsecure()}); err != nil {
		return err
	}
	pb.RegisterPyreServiceServer(gsrv, srv)
	mux.Handle("/b2api/v1/", rmux)
	go gsrv.Serve(l)
	go func() {
		<-ctx.Done()
		gsrv.GracefulStop()
	}()
	return nil
}

type AccountManager interface {
	Authorize(acct, key string) (string, error)
	CheckCreds(token, api string) error
	APIRoot(acct string) string
	DownloadRoot(acct string) string
	UploadPartHost(fileID string) (string, error)
	UploadHost(id string) (string, error)
	Sizes(acct string) (recommended, minimum int32)
}

type BucketManager interface {
	AddBucket(id, name string, bs []byte) error
	RemoveBucket(id string) error
	UpdateBucket(id string, rev int, bs []byte) error
	ListBuckets(acct string) ([][]byte, error)
	GetBucket(id string) ([]byte, error)
}

type LargeFileOrganizer interface {
	Start(bucketID, fileName, fileID string, bs []byte) error
	Get(fileID string) ([]byte, error)
	Parts(fileID string) ([]string, error)
	Finish(fileID string) error
}

type Server struct {
	Account   AccountManager
	Bucket    BucketManager
	LargeFile LargeFileOrganizer
	List      ListManager
}

func (s *Server) AuthorizeAccount(ctx context.Context, req *pb.AuthorizeAccountRequest) (*pb.AuthorizeAccountResponse, error) {
	auth, err := getAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(auth, "Basic ") {
		return nil, errors.New("basic auth required")
	}
	auth = strings.TrimPrefix(auth, "Basic ")
	bs, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return nil, err
	}
	split := strings.Split(string(bs), ":")
	if len(split) != 2 {
		return nil, errors.New("bad auth")
	}
	acct, key := split[0], split[1]
	token, err := s.Account.Authorize(acct, key)
	if err != nil {
		return nil, err
	}
	rec, min := s.Account.Sizes(acct)
	return &pb.AuthorizeAccountResponse{
		AuthorizationToken:      token,
		ApiUrl:                  s.Account.APIRoot(acct),
		DownloadUrl:             s.Account.DownloadRoot(acct),
		RecommendedPartSize:     rec,
		MinimumPartSize:         rec,
		AbsoluteMinimumPartSize: min,
	}, nil
}

func (s *Server) ListBuckets(ctx context.Context, req *pb.ListBucketsRequest) (*pb.ListBucketsResponse, error) {
	resp := &pb.ListBucketsResponse{}
	buckets, err := s.Bucket.ListBuckets(req.AccountId)
	if err != nil {
		return nil, err
	}
	for _, bs := range buckets {
		var bucket pb.Bucket
		if err := proto.Unmarshal(bs, &bucket); err != nil {
			return nil, err
		}
		resp.Buckets = append(resp.Buckets, &bucket)
	}
	return resp, nil
}

func (s *Server) CreateBucket(ctx context.Context, req *pb.Bucket) (*pb.Bucket, error) {
	req.BucketId = uuid.New().String()
	bs, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := s.Bucket.AddBucket(req.BucketId, req.BucketName, bs); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Server) DeleteBucket(ctx context.Context, req *pb.Bucket) (*pb.Bucket, error) {
	bs, err := s.Bucket.GetBucket(req.BucketId)
	if err != nil {
		return nil, err
	}
	var bucket pb.Bucket
	if err := proto.Unmarshal(bs, &bucket); err != nil {
		return nil, err
	}
	if err := s.Bucket.RemoveBucket(req.BucketId); err != nil {
		return nil, err
	}
	return &bucket, nil
}

func (s *Server) GetUploadUrl(ctx context.Context, req *pb.GetUploadUrlRequest) (*pb.GetUploadUrlResponse, error) {
	host, err := s.Account.UploadHost(req.BucketId)
	if err != nil {
		return nil, err
	}
	return &pb.GetUploadUrlResponse{
		UploadUrl: fmt.Sprintf("%s/b2api/v1/b2_upload_file/%s", host, req.BucketId),
		BucketId:  req.BucketId,
	}, nil
}

func (s *Server) StartLargeFile(ctx context.Context, req *pb.StartLargeFileRequest) (*pb.StartLargeFileResponse, error) {
	fileID := uuid.New().String()
	resp := &pb.StartLargeFileResponse{
		FileId:      fileID,
		FileName:    req.FileName,
		BucketId:    req.BucketId,
		ContentType: req.ContentType,
		FileInfo:    req.FileInfo,
	}
	bs, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}
	if err := s.LargeFile.Start(req.BucketId, req.FileName, fileID, bs); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Server) GetUploadPartUrl(ctx context.Context, req *pb.GetUploadPartUrlRequest) (*pb.GetUploadPartUrlResponse, error) {
	host, err := s.Account.UploadPartHost(req.FileId)
	if err != nil {
		return nil, err
	}
	return &pb.GetUploadPartUrlResponse{
		UploadUrl: fmt.Sprintf("%s/b2api/v1/b2_upload_part/%s", host, req.FileId),
	}, nil
}

func (s *Server) FinishLargeFile(ctx context.Context, req *pb.FinishLargeFileRequest) (*pb.FinishLargeFileResponse, error) {
	parts, err := s.LargeFile.Parts(req.FileId)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(parts, req.PartSha1Array) {
		return nil, errors.New("sha1 array mismatch")
	}
	if err := s.LargeFile.Finish(req.FileId); err != nil {
		return nil, err
	}
	return &pb.FinishLargeFileResponse{}, nil
}

func (s *Server) ListFileVersions(ctx context.Context, req *pb.ListFileVersionsRequest) (*pb.ListFileVersionsResponse, error) {
	return nil, nil
}

type objTuple struct {
	name, version string
}

type ListManager interface {
	// NextN returns the next n objects, sorted by lexicographical order by name,
	// beginning at and including, if it exists, fileName.  If withPrefix is not
	// empty, it only returns names that begin with that prefix.  If skipPrefix
	// is not empty, then the no files with that prefix are returned.  If the two
	// conflict, skipPrefix wins (i.e., do not return the entry).
	//
	// If fewer than n entries are returned, this signifies that no more names
	// exist that meet these criteria.
	NextN(bucketID, fileName, withPrefix, skipPrefix string, n int) ([]VersionedObject, error)
}

type VersionedObject interface {
	Name() string
	NextNVersions(begin string, n int) ([]string, error)
}

func getDirNames(lm ListManager, bucket, name, prefix, delim string, n int) ([]string, error) {
	var sfx string
	var out []string
	for n > 0 {
		vo, err := lm.NextN(bucket, name, prefix, sfx, 1)
		if err != nil {
			return nil, err
		}
		if len(vo) == 0 {
			return out, nil
		}
		v := vo[0]
		name = v.Name()
		suffix := name[len(prefix):]
		i := strings.Index(suffix, delim)
		if i < 0 {
			sfx = ""
			out = append(out, name)
			name += "\000"
			n--
			continue
		}
		sfx = v.Name()[:len(prefix)+i+1]
		out = append(out, sfx)
		n--
	}
	return out, nil
}

//func getNextObjects(lm ListManager, bucket, name, prefix, delimiter string, n int) ([]VersionedObject, error) {
//	if delimiter == "" {
//		return lm.NextN(bucket, name, prefix, "", n)
//	}
//	afterPfx := strings.TrimPrefix(name, prefix)
//	i := strings.Index(afterPfx, delimiter)
//	if i == 0 {
//
//	}
//	if i < 0 {
//		return lm.NextN(bucket, name, prefix, "", n)
//	}
//	skipPfx := name[:len(prefix)+i]
//	// TO
//}
//
//func listFileVersions(lm ListManager, bucket, name, version, prefix, delimiter string, n int) ([]objTuple, error) {
//	var tups []objTuple
//	var got int
//	for {
//		objs, err := getNextObjects(bucket, name, prefix, delimiter, n-got)
//		if err != nil {
//			return nil, err
//		}
//		if len(objs) == 0 {
//			break
//		}
//		for _, o := range objs {
//			var begin string
//			if len(tups) == 0 {
//				begin = version
//			}
//			vers, err := lm.NextNVersions(begin, n-got)
//			if err != nil {
//				return nil, err
//			}
//			got += len(vers)
//			for _, ver := range vers {
//				tups = append(tups, objTuple{name: o.Name(), version: ver})
//			}
//			if got >= n {
//				return tups[:n], nil
//			}
//		}
//	}
//	return tups, nil
//}
