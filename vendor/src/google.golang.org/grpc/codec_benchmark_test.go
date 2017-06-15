// +build go1.7

/*
 *
 * Copyright 2014 gRPC authors.
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

package grpc

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/test/codec_perf"
)

func setupBenchmarkProtoCodecInputs(b *testing.B, payloadBaseSize uint32) []proto.Message {
	payloadBase := make([]byte, payloadBaseSize)
	// arbitrary byte slices
	payloadSuffixes := [][]byte{
		[]byte("one"),
		[]byte("two"),
		[]byte("three"),
		[]byte("four"),
		[]byte("five"),
	}
	protoStructs := make([]proto.Message, 0)

	for _, p := range payloadSuffixes {
		ps := &codec_perf.Buffer{}
		ps.Body = append(payloadBase, p...)
		protoStructs = append(protoStructs, ps)
	}

	return protoStructs
}

// The possible use of certain protobuf APIs like the proto.Buffer API potentially involves caching
// on our side. This can add checks around memory allocations and possible contention.
// Example run: go test -v -run=^$ -bench=BenchmarkProtoCodec -benchmem
func BenchmarkProtoCodec(b *testing.B) {
	// range of message sizes
	payloadBaseSizes := make([]uint32, 0)
	for i := uint32(0); i <= 12; i += 4 {
		payloadBaseSizes = append(payloadBaseSizes, 1<<i)
	}
	// range of SetParallelism
	parallelisms := make([]uint32, 0)
	for i := uint32(0); i <= 16; i += 4 {
		parallelisms = append(parallelisms, 1<<i)
	}
	for _, s := range payloadBaseSizes {
		for _, p := range parallelisms {
			func(parallelism int, payloadBaseSize uint32) {
				protoStructs := setupBenchmarkProtoCodecInputs(b, payloadBaseSize)
				name := fmt.Sprintf("MinPayloadSize:%v/SetParallelism(%v)", payloadBaseSize, parallelism)
				b.Run(name, func(b *testing.B) {
					codec := &protoCodec{}
					b.SetParallelism(parallelism)
					b.RunParallel(func(pb *testing.PB) {
						benchmarkProtoCodec(codec, protoStructs, pb, b)
					})
				})
			}(int(p), s)
		}
	}
}

func benchmarkProtoCodec(codec *protoCodec, protoStructs []proto.Message, pb *testing.PB, b *testing.B) {
	counter := 0
	for pb.Next() {
		counter++
		ps := protoStructs[counter%len(protoStructs)]
		fastMarshalAndUnmarshal(codec, ps, b)
	}
}

func fastMarshalAndUnmarshal(protoCodec Codec, protoStruct proto.Message, b *testing.B) {
	marshaledBytes, err := protoCodec.Marshal(protoStruct)
	if err != nil {
		b.Errorf("protoCodec.Marshal(_) returned an error")
	}
	if err := protoCodec.Unmarshal(marshaledBytes, protoStruct); err != nil {
		b.Errorf("protoCodec.Unmarshal(_) returned an error")
	}
}
