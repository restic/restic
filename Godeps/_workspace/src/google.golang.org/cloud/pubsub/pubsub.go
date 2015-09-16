// Copyright 2014 Google Inc. All Rights Reserved.
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

// Package pubsub contains a Google Cloud Pub/Sub client.
//
// This package is experimental and may make backwards-incompatible changes.
//
// More information about Google Cloud Pub/Sub is available at
// https://cloud.google.com/pubsub/docs
package pubsub

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/cloud/internal"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/pubsub/v1beta2"
)

const (
	// ScopePubSub grants permissions to view and manage Pub/Sub
	// topics and subscriptions.
	ScopePubSub = "https://www.googleapis.com/auth/pubsub"

	// ScopeCloudPlatform grants permissions to view and manage your data
	// across Google Cloud Platform services.
	ScopeCloudPlatform = "https://www.googleapis.com/auth/cloud-platform"
)

// batchLimit is maximun size of a single batch.
const batchLimit = 1000

// Message represents a Pub/Sub message.
type Message struct {
	// ID identifies this message.
	ID string

	// AckID is the identifier to acknowledge this message.
	AckID string

	// Data is the actual data in the message.
	Data []byte

	// Attributes represents the key-value pairs the current message
	// is labelled with.
	Attributes map[string]string
}

// TODO(jbd): Add subscription and topic listing.

// CreateSub creates a Pub/Sub subscription on the backend.
// A subscription should subscribe to an existing topic.
//
// The messages that haven't acknowledged will be pushed back to the
// subscription again when the default acknowledgement deadline is
// reached. You can override the default deadline by providing a
// non-zero deadline. Deadline must not be specified to
// precision greater than one second.
//
// As new messages are being queued on the subscription, you
// may recieve push notifications regarding to the new arrivals.
// To receive notifications of new messages in the queue,
// specify an endpoint callback URL.
// If endpoint is an empty string the backend will not notify the
// client of new messages.
//
// If the subscription already exists an error will be returned.
func CreateSub(ctx context.Context, name string, topic string, deadline time.Duration, endpoint string) error {
	sub := &raw.Subscription{
		Topic: fullTopicName(internal.ProjID(ctx), topic),
	}
	if int64(deadline) > 0 {
		if !isSec(deadline) {
			return errors.New("pubsub: deadline must not be specified to precision greater than one second")
		}
		sub.AckDeadlineSeconds = int64(deadline / time.Second)
	}
	if endpoint != "" {
		sub.PushConfig = &raw.PushConfig{PushEndpoint: endpoint}
	}
	_, err := rawService(ctx).Projects.Subscriptions.Create(fullSubName(internal.ProjID(ctx), name), sub).Do()
	return err
}

// DeleteSub deletes the subscription.
func DeleteSub(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Subscriptions.Delete(fullSubName(internal.ProjID(ctx), name)).Do()
	return err
}

// ModifyAckDeadline modifies the acknowledgement deadline
// for the messages retrieved from the specified subscription.
// Deadline must not be specified to precision greater than one second.
func ModifyAckDeadline(ctx context.Context, sub string, id string, deadline time.Duration) error {
	if !isSec(deadline) {
		return errors.New("pubsub: deadline must not be specified to precision greater than one second")
	}
	_, err := rawService(ctx).Projects.Subscriptions.ModifyAckDeadline(fullSubName(internal.ProjID(ctx), sub), &raw.ModifyAckDeadlineRequest{
		AckDeadlineSeconds: int64(deadline / time.Second),
		AckId:              id,
	}).Do()
	return err
}

// ModifyPushEndpoint modifies the URL endpoint to modify the resource
// to handle push notifications coming from the Pub/Sub backend
// for the specified subscription.
func ModifyPushEndpoint(ctx context.Context, sub, endpoint string) error {
	_, err := rawService(ctx).Projects.Subscriptions.ModifyPushConfig(fullSubName(internal.ProjID(ctx), sub), &raw.ModifyPushConfigRequest{
		PushConfig: &raw.PushConfig{
			PushEndpoint: endpoint,
		},
	}).Do()
	return err
}

// SubExists returns true if subscription exists.
func SubExists(ctx context.Context, name string) (bool, error) {
	_, err := rawService(ctx).Projects.Subscriptions.Get(fullSubName(internal.ProjID(ctx), name)).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Ack acknowledges one or more Pub/Sub messages on the
// specified subscription.
func Ack(ctx context.Context, sub string, id ...string) error {
	for idx, ackID := range id {
		if ackID == "" {
			return fmt.Errorf("pubsub: empty ackID detected at index %d", idx)
		}
	}
	_, err := rawService(ctx).Projects.Subscriptions.Acknowledge(fullSubName(internal.ProjID(ctx), sub), &raw.AcknowledgeRequest{
		AckIds: id,
	}).Do()
	return err
}

func toMessage(resp *raw.ReceivedMessage) (*Message, error) {
	if resp.Message == nil {
		return &Message{AckID: resp.AckId}, nil
	}
	data, err := base64.StdEncoding.DecodeString(resp.Message.Data)
	if err != nil {
		return nil, err
	}
	return &Message{
		AckID:      resp.AckId,
		Data:       data,
		Attributes: resp.Message.Attributes,
		ID:         resp.Message.MessageId,
	}, nil
}

// Pull pulls messages from the subscription. It returns up to n
// number of messages, and n could not be larger than 100.
func Pull(ctx context.Context, sub string, n int) ([]*Message, error) {
	return pull(ctx, sub, n, true)
}

// PullWait pulls messages from the subscription. If there are not
// enough messages left in the subscription queue, it will block until
// at least n number of messages arrive or timeout occurs, and n could
// not be larger than 100.
func PullWait(ctx context.Context, sub string, n int) ([]*Message, error) {
	return pull(ctx, sub, n, false)
}

func pull(ctx context.Context, sub string, n int, retImmediately bool) ([]*Message, error) {
	if n < 1 || n > batchLimit {
		return nil, fmt.Errorf("pubsub: cannot pull less than one, more than %d messages, but %d was given", batchLimit, n)
	}
	resp, err := rawService(ctx).Projects.Subscriptions.Pull(fullSubName(internal.ProjID(ctx), sub), &raw.PullRequest{
		ReturnImmediately: retImmediately,
		MaxMessages:       int64(n),
	}).Do()
	if err != nil {
		return nil, err
	}
	msgs := make([]*Message, len(resp.ReceivedMessages))
	for i := 0; i < len(resp.ReceivedMessages); i++ {
		msg, err := toMessage(resp.ReceivedMessages[i])
		if err != nil {
			return nil, fmt.Errorf("pubsub: cannot decode the retrieved message at index: %d, PullResponse: %+v", i, resp.ReceivedMessages[i])
		}
		msgs[i] = msg
	}
	return msgs, nil
}

// CreateTopic creates a new topic with the specified name on the backend.
// It will return an error if topic already exists.
func CreateTopic(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Topics.Create(fullTopicName(internal.ProjID(ctx), name), &raw.Topic{}).Do()
	return err
}

// DeleteTopic deletes the specified topic.
func DeleteTopic(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Topics.Delete(fullTopicName(internal.ProjID(ctx), name)).Do()
	return err
}

// TopicExists returns true if a topic exists with the specified name.
func TopicExists(ctx context.Context, name string) (bool, error) {
	_, err := rawService(ctx).Projects.Topics.Get(fullTopicName(internal.ProjID(ctx), name)).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Publish publish messages to the topic's subscribers. It returns
// message IDs upon success.
func Publish(ctx context.Context, topic string, msgs ...*Message) ([]string, error) {
	var rawMsgs []*raw.PubsubMessage
	if len(msgs) == 0 {
		return nil, errors.New("pubsub: no messages to publish")
	}
	if len(msgs) > batchLimit {
		return nil, fmt.Errorf("pubsub: %d messages given, but maximum batch size is %d", len(msgs), batchLimit)
	}
	rawMsgs = make([]*raw.PubsubMessage, len(msgs))
	for i, msg := range msgs {
		rawMsgs[i] = &raw.PubsubMessage{
			Data:       base64.StdEncoding.EncodeToString(msg.Data),
			Attributes: msg.Attributes,
		}
	}
	resp, err := rawService(ctx).Projects.Topics.Publish(fullTopicName(internal.ProjID(ctx), topic), &raw.PublishRequest{
		Messages: rawMsgs,
	}).Do()
	if err != nil {
		return nil, err
	}
	return resp.MessageIds, nil
}

// fullSubName returns the fully qualified name for a subscription.
// E.g. /subscriptions/project-id/subscription-name.
func fullSubName(proj, name string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", proj, name)
}

// fullTopicName returns the fully qualified name for a topic.
// E.g. /topics/project-id/topic-name.
func fullTopicName(proj, name string) string {
	return fmt.Sprintf("projects/%s/topics/%s", proj, name)
}

func isSec(dur time.Duration) bool {
	return dur%time.Second == 0
}

func rawService(ctx context.Context) *raw.Service {
	return internal.Service(ctx, "pubsub", func(hc *http.Client) interface{} {
		svc, _ := raw.New(hc)
		return svc
	}).(*raw.Service)
}
