//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//

package aws

import (
	"errors"
	"os"
	"strings"
)

// Region defines the URLs where AWS services may be accessed.
//
// See http://goo.gl/d8BP1 for more details.
type Region struct {
	Name                 string // the canonical name of this region.
	EC2Endpoint          string
	S3Endpoint           string
	S3BucketEndpoint     string // Not needed by AWS S3. Use ${bucket} for bucket name.
	S3LocationConstraint bool   // true if this region requires a LocationConstraint declaration.
	S3LowercaseBucket    bool   // true if the region requires bucket names to be lower case.
	SDBEndpoint          string // not all regions have simpleDB, fro eg. Frankfurt (eu-central-1) does not
	SNSEndpoint          string
	SQSEndpoint          string
	IAMEndpoint          string
}

func (r Region) ResolveS3BucketEndpoint(bucketName string) string {
	if r.S3BucketEndpoint != "" {
		return strings.ToLower(strings.Replace(r.S3BucketEndpoint, "${bucket}", bucketName, -1))
	}
	return strings.ToLower(r.S3Endpoint + "/" + bucketName + "/")
}

var USEast = Region{
	"us-east-1", // US East (N. Virginia)
	"https://ec2.us-east-1.amazonaws.com",
	"https://s3.amazonaws.com",
	"",
	false,
	false,
	"https://sdb.amazonaws.com",
	"https://sns.us-east-1.amazonaws.com",
	"https://sqs.us-east-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var USWest = Region{
	"us-west-1", //US West (N. California)
	"https://ec2.us-west-1.amazonaws.com",
	"https://s3-us-west-1.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.us-west-1.amazonaws.com",
	"https://sns.us-west-1.amazonaws.com",
	"https://sqs.us-west-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var USWest2 = Region{
	"us-west-2", // US West (Oregon)
	"https://ec2.us-west-2.amazonaws.com",
	"https://s3-us-west-2.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.us-west-2.amazonaws.com",
	"https://sns.us-west-2.amazonaws.com",
	"https://sqs.us-west-2.amazonaws.com",
	"https://iam.amazonaws.com",
}

var USGovWest = Region{
	"us-gov-west-1", // Isolated regions, AWS GovCloud (US)
	"https://ec2.us-gov-west-1.amazonaws.com",
	"https://s3-us-gov-west-1.amazonaws.com",
	"",
	true,
	true,
	"",
	"https://sns.us-gov-west-1.amazonaws.com",
	"https://sqs.us-gov-west-1.amazonaws.com",
	"https://iam.us-gov.amazonaws.com",
}

var EUWest = Region{
	"eu-west-1", // EU (Ireland)
	"https://ec2.eu-west-1.amazonaws.com",
	"https://s3-eu-west-1.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.eu-west-1.amazonaws.com",
	"https://sns.eu-west-1.amazonaws.com",
	"https://sqs.eu-west-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var EUCentral = Region{
	"eu-central-1", // EU (Frankfurt)
	"https://ec2.eu-central-1.amazonaws.com",
	"https://s3-eu-central-1.amazonaws.com",
	"",
	true,
	true,
	"",
	"https://sns.eu-central-1.amazonaws.com",
	"https://sqs.eu-central-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var APSoutheast = Region{
	"ap-southeast-1", // Asia Pacific (Singapore)
	"https://ec2.ap-southeast-1.amazonaws.com",
	"https://s3-ap-southeast-1.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.ap-southeast-1.amazonaws.com",
	"https://sns.ap-southeast-1.amazonaws.com",
	"https://sqs.ap-southeast-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var APSoutheast2 = Region{
	"ap-southeast-2", //Asia Pacific (Sydney)
	"https://ec2.ap-southeast-2.amazonaws.com",
	"https://s3-ap-southeast-2.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.ap-southeast-2.amazonaws.com",
	"https://sns.ap-southeast-2.amazonaws.com",
	"https://sqs.ap-southeast-2.amazonaws.com",
	"https://iam.amazonaws.com",
}

var APNortheast = Region{
	"ap-northeast-1", //Asia Pacific (Tokyo)
	"https://ec2.ap-northeast-1.amazonaws.com",
	"https://s3-ap-northeast-1.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.ap-northeast-1.amazonaws.com",
	"https://sns.ap-northeast-1.amazonaws.com",
	"https://sqs.ap-northeast-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var SAEast = Region{
	"sa-east-1", // South America (Sao Paulo)
	"https://ec2.sa-east-1.amazonaws.com",
	"https://s3-sa-east-1.amazonaws.com",
	"",
	true,
	true,
	"https://sdb.sa-east-1.amazonaws.com",
	"https://sns.sa-east-1.amazonaws.com",
	"https://sqs.sa-east-1.amazonaws.com",
	"https://iam.amazonaws.com",
}

var CNNorth = Region{
	"cn-north-1", // Isolated regions, China (Beijing)
	"https://ec2.cn-north-1.amazonaws.com.cn",
	"https://s3.cn-north-1.amazonaws.com.cn",
	"",
	true,
	true,
	"https://sdb.cn-north-1.amazonaws.com.cn",
	"https://sns.cn-north-1.amazonaws.com.cn",
	"https://sqs.cn-north-1.amazonaws.com.cn",
	"https://iam.cn-north-1.amazonaws.com.cn",
}

var Regions = map[string]Region{
	APNortheast.Name:  APNortheast,
	APSoutheast.Name:  APSoutheast,
	APSoutheast2.Name: APSoutheast2,
	EUWest.Name:       EUWest,
	EUCentral.Name:    EUCentral,
	USEast.Name:       USEast,
	USWest.Name:       USWest,
	USWest2.Name:      USWest2,
	USGovWest.Name:    USGovWest,
	SAEast.Name:       SAEast,
	CNNorth.Name:      CNNorth,
}

type Auth struct {
	AccessKey, SecretKey string
}

var unreserved = make([]bool, 128)
var hex = "0123456789ABCDEF"

func init() {
	// RFC3986
	u := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz01234567890-_.~"
	for _, c := range u {
		unreserved[c] = true
	}
}

// EnvAuth creates an Auth based on environment information.
// The AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment
// variables are used as the first preference, but EC2_ACCESS_KEY
// and EC2_SECRET_KEY or AWS_ACCESS_KEY and AWS_SECRET_KEY
// environment variables are also supported.
func EnvAuth() (auth Auth, err error) {
	auth.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	auth.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	// first fallbaback to EC2_ env variable
	if auth.AccessKey == "" && auth.SecretKey == "" {
		auth.AccessKey = os.Getenv("EC2_ACCESS_KEY")
		auth.SecretKey = os.Getenv("EC2_SECRET_KEY")
	}
	// second fallbaback to AWS_ env variable
	if auth.AccessKey == "" && auth.SecretKey == "" {
		auth.AccessKey = os.Getenv("AWS_ACCESS_KEY")
		auth.SecretKey = os.Getenv("AWS_SECRET_KEY")
	}
	if auth.AccessKey == "" {
		err = errors.New("AWS_ACCESS_KEY_ID not found in environment")
	}
	if auth.SecretKey == "" {
		err = errors.New("AWS_SECRET_ACCESS_KEY not found in environment")
	}
	return
}

// Encode takes a string and URI-encodes it in a way suitable
// to be used in AWS signatures.
func Encode(s string) string {
	encode := false
	for i := 0; i != len(s); i++ {
		c := s[i]
		if c > 127 || !unreserved[c] {
			encode = true
			break
		}
	}
	if !encode {
		return s
	}
	e := make([]byte, len(s)*3)
	ei := 0
	for i := 0; i != len(s); i++ {
		c := s[i]
		if c > 127 || !unreserved[c] {
			e[ei] = '%'
			e[ei+1] = hex[c>>4]
			e[ei+2] = hex[c&0xF]
			ei += 3
		} else {
			e[ei] = c
			ei += 1
		}
	}
	return string(e[:ei])
}
