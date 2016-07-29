/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage (C) 2016 Minio, Inc.
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
 */

package minio

import (
	"encoding/xml"
)

// S3 notification events
type Event string

const (
	ObjectCreatedAll                     Event = "s3:ObjectCreated:*"
	ObjectCreatePut                            = "s3:ObjectCreated:Put"
	ObjectCreatedPost                          = "s3:ObjectCreated:Post"
	ObjectCreatedCopy                          = "s3:ObjectCreated:Copy"
	ObjectCreatedCompleteMultipartUpload       = "sh:ObjectCreated:CompleteMultipartUpload"
	ObjectRemovedAll                           = "s3:ObjectRemoved:*"
	ObjectRemovedDelete                        = "s3:ObjectRemoved:Delete"
	ObjectRemovedDeleteMarkerCreated           = "s3:ObjectRemoved:DeleteMarkerCreated"
	ObjectReducedRedundancyLostObject          = "s3:ReducedRedundancyLostObject"
)

type FilterRule struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

type S3Key struct {
	FilterRules []FilterRule `xml:"FilterRule,omitempty"`
}

type Filter struct {
	S3Key S3Key `xml:"S3Key,omitempty"`
}

// Arn - holds ARN information that will be sent to the web service
type Arn struct {
	Partition string
	Service   string
	Region    string
	AccountID string
	Resource  string
}

func NewArn(partition, service, region, accountID, resource string) Arn {
	return Arn{Partition: partition,
		Service:   service,
		Region:    region,
		AccountID: accountID,
		Resource:  resource}
}

func (arn Arn) String() string {
	return "arn:" + arn.Partition + ":" + arn.Service + ":" + arn.Region + ":" + arn.AccountID + ":" + arn.Resource
}

// NotificationConfig - represents one single notification configuration
// such as topic, queue or lambda configuration.
type NotificationConfig struct {
	Id     string  `xml:"Id,omitempty"`
	Arn    Arn     `xml:"-"`
	Events []Event `xml:"Event"`
	Filter *Filter `xml:"Filter,omitempty"`
}

func NewNotificationConfig(arn Arn) NotificationConfig {
	return NotificationConfig{Arn: arn}
}

func (t *NotificationConfig) AddEvents(events ...Event) {
	t.Events = append(t.Events, events...)
}

func (t *NotificationConfig) AddFilterSuffix(suffix string) {
	if t.Filter == nil {
		t.Filter = &Filter{}
	}
	t.Filter.S3Key.FilterRules = append(t.Filter.S3Key.FilterRules, FilterRule{Name: "suffix", Value: suffix})
}

func (t *NotificationConfig) AddFilterPrefix(prefix string) {
	if t.Filter == nil {
		t.Filter = &Filter{}
	}
	t.Filter.S3Key.FilterRules = append(t.Filter.S3Key.FilterRules, FilterRule{Name: "prefix", Value: prefix})
}

// Topic notification config
type TopicConfig struct {
	NotificationConfig
	Topic string `xml:"Topic"`
}

type QueueConfig struct {
	NotificationConfig
	Queue string `xml:"Queue"`
}

type LambdaConfig struct {
	NotificationConfig
	Lambda string `xml:"CloudFunction"`
}

// BucketNotification - the struct that represents the whole XML to be sent to the web service
type BucketNotification struct {
	XMLName       xml.Name       `xml:"NotificationConfiguration"`
	LambdaConfigs []LambdaConfig `xml:"CloudFunctionConfiguration"`
	TopicConfigs  []TopicConfig  `xml:"TopicConfiguration"`
	QueueConfigs  []QueueConfig  `xml:"QueueConfiguration"`
}

func (b *BucketNotification) AddTopic(topicConfig NotificationConfig) {
	config := TopicConfig{NotificationConfig: topicConfig, Topic: topicConfig.Arn.String()}
	b.TopicConfigs = append(b.TopicConfigs, config)
}

func (b *BucketNotification) AddQueue(queueConfig NotificationConfig) {
	config := QueueConfig{NotificationConfig: queueConfig, Queue: queueConfig.Arn.String()}
	b.QueueConfigs = append(b.QueueConfigs, config)
}

func (b *BucketNotification) AddLambda(lambdaConfig NotificationConfig) {
	config := LambdaConfig{NotificationConfig: lambdaConfig, Lambda: lambdaConfig.Arn.String()}
	b.LambdaConfigs = append(b.LambdaConfigs, config)
}
