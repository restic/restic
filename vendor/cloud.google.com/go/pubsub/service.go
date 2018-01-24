// Copyright 2016 Google Inc. All Rights Reserved.
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

package pubsub

import (
	"fmt"
	"math"
	"strings"

	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxPayload is the maximum number of bytes to devote to actual ids in
// acknowledgement or modifyAckDeadline requests. A serialized
// AcknowledgeRequest proto has a small constant overhead, plus the size of the
// subscription name, plus 3 bytes per ID (a tag byte and two size bytes). A
// ModifyAckDeadlineRequest has an additional few bytes for the deadline. We
// don't know the subscription name here, so we just assume the size exclusive
// of ids is 100 bytes.
//
// With gRPC there is no way for the client to know the server's max message size (it is
// configurable on the server). We know from experience that it
// it 512K.
const (
	maxPayload       = 512 * 1024
	reqFixedOverhead = 100
	overheadPerID    = 3
	maxSendRecvBytes = 20 * 1024 * 1024 // 20M
)

func convertMessages(rms []*pb.ReceivedMessage) ([]*Message, error) {
	msgs := make([]*Message, 0, len(rms))
	for i, m := range rms {
		msg, err := toMessage(m)
		if err != nil {
			return nil, fmt.Errorf("pubsub: cannot decode the retrieved message at index: %d, message: %+v", i, m)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func trunc32(i int64) int32 {
	if i > math.MaxInt32 {
		i = math.MaxInt32
	}
	return int32(i)
}

// func newStreamingPuller(ctx context.Context, subc *vkit.SubscriberClient, subName string, ackDeadlineSecs int32) *streamingPuller {
// 	p := &streamingPuller{
// 		ctx:             ctx,
// 		subName:         subName,
// 		ackDeadlineSecs: ackDeadlineSecs,
// 		subc:            subc,
// 	}
// 	p.c = sync.NewCond(&p.mu)
// 	return p
// }

// type streamingPuller struct {
// 	ctx             context.Context
// 	subName         string
// 	ackDeadlineSecs int32
// 	subc            *vkit.SubscriberClient

// 	mu       sync.Mutex
// 	c        *sync.Cond
// 	inFlight bool
// 	closed   bool // set after CloseSend called
// 	spc      pb.Subscriber_StreamingPullClient
// 	err      error
// }

// // open establishes (or re-establishes) a stream for pulling messages.
// // It takes care that only one RPC is in flight at a time.
// func (p *streamingPuller) open() error {
// 	p.c.L.Lock()
// 	defer p.c.L.Unlock()
// 	p.openLocked()
// 	return p.err
// }

// func (p *streamingPuller) openLocked() {
// 	if p.inFlight {
// 		// Another goroutine is opening; wait for it.
// 		for p.inFlight {
// 			p.c.Wait()
// 		}
// 		return
// 	}
// 	// No opens in flight; start one.
// 	// Keep the lock held, to avoid a race where we
// 	// close the old stream while opening a new one.
// 	p.inFlight = true
// 	spc, err := p.subc.StreamingPull(p.ctx, gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(maxSendRecvBytes)))
// 	if err == nil {
// 		err = spc.Send(&pb.StreamingPullRequest{
// 			Subscription:             p.subName,
// 			StreamAckDeadlineSeconds: p.ackDeadlineSecs,
// 		})
// 	}
// 	p.spc = spc
// 	p.err = err
// 	p.inFlight = false
// 	p.c.Broadcast()
// }

// func (p *streamingPuller) call(f func(pb.Subscriber_StreamingPullClient) error) error {
// 	p.c.L.Lock()
// 	defer p.c.L.Unlock()
// 	// Wait for an open in flight.
// 	for p.inFlight {
// 		p.c.Wait()
// 	}
// 	var err error
// 	var bo gax.Backoff
// 	for {
// 		select {
// 		case <-p.ctx.Done():
// 			p.err = p.ctx.Err()
// 		default:
// 		}
// 		if p.err != nil {
// 			return p.err
// 		}
// 		spc := p.spc
// 		// Do not call f with the lock held. Only one goroutine calls Send
// 		// (streamingMessageIterator.sender) and only one calls Recv
// 		// (streamingMessageIterator.receiver). If we locked, then a
// 		// blocked Recv would prevent a Send from happening.
// 		p.c.L.Unlock()
// 		err = f(spc)
// 		p.c.L.Lock()
// 		if !p.closed && err != nil && isRetryable(err) {
// 			// Sleep with exponential backoff. Normally we wouldn't hold the lock while sleeping,
// 			// but here it can't do any harm, since the stream is broken anyway.
// 			gax.Sleep(p.ctx, bo.Pause())
// 			p.openLocked()
// 			continue
// 		}
// 		// Not an error, or not a retryable error; stop retrying.
// 		p.err = err
// 		return err
// 	}
// }

// Logic from https://github.com/GoogleCloudPlatform/google-cloud-java/blob/master/google-cloud-pubsub/src/main/java/com/google/cloud/pubsub/v1/StatusUtil.java.
func isRetryable(err error) bool {
	s, ok := status.FromError(err)
	if !ok { // includes io.EOF, normal stream close, which causes us to reopen
		return true
	}
	switch s.Code() {
	case codes.DeadlineExceeded, codes.Internal, codes.Canceled, codes.ResourceExhausted:
		return true
	case codes.Unavailable:
		return !strings.Contains(s.Message(), "Server shutdownNow invoked")
	default:
		return false
	}
}

// func (p *streamingPuller) fetchMessages() ([]*Message, error) {
// 	var res *pb.StreamingPullResponse
// 	err := p.call(func(spc pb.Subscriber_StreamingPullClient) error {
// 		var err error
// 		res, err = spc.Recv()
// 		return err
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	return convertMessages(res.ReceivedMessages)
// }

// func (p *streamingPuller) send(req *pb.StreamingPullRequest) error {
// 	// Note: len(modAckIDs) == len(modSecs)
// 	var rest *pb.StreamingPullRequest
// 	for len(req.AckIds) > 0 || len(req.ModifyDeadlineAckIds) > 0 {
// 		req, rest = splitRequest(req, maxPayload)
// 		err := p.call(func(spc pb.Subscriber_StreamingPullClient) error {
// 			x := spc.Send(req)
// 			return x
// 		})
// 		if err != nil {
// 			return err
// 		}
// 		req = rest
// 	}
// 	return nil
// }

// func (p *streamingPuller) closeSend() {
// 	p.mu.Lock()
// 	p.closed = true
// 	p.spc.CloseSend()
// 	p.mu.Unlock()
// }

// Split req into a prefix that is smaller than maxSize, and a remainder.
func splitRequest(req *pb.StreamingPullRequest, maxSize int) (prefix, remainder *pb.StreamingPullRequest) {
	const int32Bytes = 4

	// Copy all fields before splitting the variable-sized ones.
	remainder = &pb.StreamingPullRequest{}
	*remainder = *req
	// Split message so it isn't too big.
	size := reqFixedOverhead
	i := 0
	for size < maxSize && (i < len(req.AckIds) || i < len(req.ModifyDeadlineAckIds)) {
		if i < len(req.AckIds) {
			size += overheadPerID + len(req.AckIds[i])
		}
		if i < len(req.ModifyDeadlineAckIds) {
			size += overheadPerID + len(req.ModifyDeadlineAckIds[i]) + int32Bytes
		}
		i++
	}

	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	j := i
	if size > maxSize {
		j--
	}
	k := min(j, len(req.AckIds))
	remainder.AckIds = req.AckIds[k:]
	req.AckIds = req.AckIds[:k]
	k = min(j, len(req.ModifyDeadlineAckIds))
	remainder.ModifyDeadlineAckIds = req.ModifyDeadlineAckIds[k:]
	remainder.ModifyDeadlineSeconds = req.ModifyDeadlineSeconds[k:]
	req.ModifyDeadlineAckIds = req.ModifyDeadlineAckIds[:k]
	req.ModifyDeadlineSeconds = req.ModifyDeadlineSeconds[:k]
	return req, remainder
}
