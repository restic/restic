package oss

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"time"
)

// ListBucketsResult defines the result object from ListBuckets request
type ListBucketsResult struct {
	XMLName     xml.Name           `xml:"ListAllMyBucketsResult"`
	Prefix      string             `xml:"Prefix"`         // The prefix in this query
	Marker      string             `xml:"Marker"`         // The marker filter
	MaxKeys     int                `xml:"MaxKeys"`        // The max entry count to return. This information is returned when IsTruncated is true.
	IsTruncated bool               `xml:"IsTruncated"`    // Flag true means there's remaining buckets to return.
	NextMarker  string             `xml:"NextMarker"`     // The marker filter for the next list call
	Owner       Owner              `xml:"Owner"`          // The owner information
	Buckets     []BucketProperties `xml:"Buckets>Bucket"` // The bucket list
}

// BucketProperties defines bucket properties
type BucketProperties struct {
	XMLName      xml.Name  `xml:"Bucket"`
	Name         string    `xml:"Name"`         // Bucket name
	Location     string    `xml:"Location"`     // Bucket datacenter
	CreationDate time.Time `xml:"CreationDate"` // Bucket create time
	StorageClass string    `xml:"StorageClass"` // Bucket storage class
}

// GetBucketACLResult defines GetBucketACL request's result
type GetBucketACLResult struct {
	XMLName xml.Name `xml:"AccessControlPolicy"`
	ACL     string   `xml:"AccessControlList>Grant"` // Bucket ACL
	Owner   Owner    `xml:"Owner"`                   // Bucket owner
}

// LifecycleConfiguration is the Bucket Lifecycle configuration
type LifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Rules   []LifecycleRule `xml:"Rule"`
}

// LifecycleRule defines Lifecycle rules
type LifecycleRule struct {
	XMLName              xml.Name                       `xml:"Rule"`
	ID                   string                         `xml:"ID,omitempty"`                   // The rule ID
	Prefix               string                         `xml:"Prefix"`                         // The object key prefix
	Status               string                         `xml:"Status"`                         // The rule status (enabled or not)
	Tags                 []Tag                          `xml:"Tag,omitempty"`                  // the tags property
	Expiration           *LifecycleExpiration           `xml:"Expiration,omitempty"`           // The expiration property
	Transitions          []LifecycleTransition          `xml:"Transition,omitempty"`           // The transition property
	AbortMultipartUpload *LifecycleAbortMultipartUpload `xml:"AbortMultipartUpload,omitempty"` // The AbortMultipartUpload property
	NonVersionExpiration *LifecycleVersionExpiration    `xml:"NoncurrentVersionExpiration,omitempty"`
	NonVersionTransition *LifecycleVersionTransition    `xml:"NoncurrentVersionTransition,omitempty"`
}

// LifecycleExpiration defines the rule's expiration property
type LifecycleExpiration struct {
	XMLName                   xml.Name `xml:"Expiration"`
	Days                      int      `xml:"Days,omitempty"`                      // Relative expiration time: The expiration time in days after the last modified time
	Date                      string   `xml:"Date,omitempty"`                      // Absolute expiration time: The expiration time in date, not recommended
	CreatedBeforeDate         string   `xml:"CreatedBeforeDate,omitempty"`         // objects created before the date will be expired
	ExpiredObjectDeleteMarker *bool    `xml:"ExpiredObjectDeleteMarker,omitempty"` // Specifies whether the expired delete tag is automatically deleted
}

// LifecycleTransition defines the rule's transition propery
type LifecycleTransition struct {
	XMLName           xml.Name         `xml:"Transition"`
	Days              int              `xml:"Days,omitempty"`              // Relative transition time: The transition time in days after the last modified time
	CreatedBeforeDate string           `xml:"CreatedBeforeDate,omitempty"` // objects created before the date will be expired
	StorageClass      StorageClassType `xml:"StorageClass,omitempty"`      // Specifies the target storage type
}

// LifecycleAbortMultipartUpload defines the rule's abort multipart upload propery
type LifecycleAbortMultipartUpload struct {
	XMLName           xml.Name `xml:"AbortMultipartUpload"`
	Days              int      `xml:"Days,omitempty"`              // Relative expiration time: The expiration time in days after the last modified time
	CreatedBeforeDate string   `xml:"CreatedBeforeDate,omitempty"` // objects created before the date will be expired
}

// LifecycleVersionExpiration defines the rule's NoncurrentVersionExpiration propery
type LifecycleVersionExpiration struct {
	XMLName        xml.Name `xml:"NoncurrentVersionExpiration"`
	NoncurrentDays int      `xml:"NoncurrentDays,omitempty"` // How many days after the Object becomes a non-current version
}

// LifecycleVersionTransition defines the rule's NoncurrentVersionTransition propery
type LifecycleVersionTransition struct {
	XMLName        xml.Name         `xml:"NoncurrentVersionTransition"`
	NoncurrentDays int              `xml:"NoncurrentDays,omitempty"` // How many days after the Object becomes a non-current version
	StorageClass   StorageClassType `xml:"StorageClass,omitempty"`
}

const iso8601DateFormat = "2006-01-02T15:04:05.000Z"

// BuildLifecycleRuleByDays builds a lifecycle rule objects will expiration in days after the last modified time
func BuildLifecycleRuleByDays(id, prefix string, status bool, days int) LifecycleRule {
	var statusStr = "Enabled"
	if !status {
		statusStr = "Disabled"
	}
	return LifecycleRule{ID: id, Prefix: prefix, Status: statusStr,
		Expiration: &LifecycleExpiration{Days: days}}
}

// BuildLifecycleRuleByDate builds a lifecycle rule objects will expiration in specified date
func BuildLifecycleRuleByDate(id, prefix string, status bool, year, month, day int) LifecycleRule {
	var statusStr = "Enabled"
	if !status {
		statusStr = "Disabled"
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Format(iso8601DateFormat)
	return LifecycleRule{ID: id, Prefix: prefix, Status: statusStr,
		Expiration: &LifecycleExpiration{Date: date}}
}

// ValidateLifecycleRule Determine if a lifecycle rule is valid, if it is invalid, it will return an error.
func verifyLifecycleRules(rules []LifecycleRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("invalid rules, the length of rules is zero")
	}
	for _, rule := range rules {
		if rule.Status != "Enabled" && rule.Status != "Disabled" {
			return fmt.Errorf("invalid rule, the value of status must be Enabled or Disabled")
		}

		abortMPU := rule.AbortMultipartUpload
		if abortMPU != nil {
			if (abortMPU.Days != 0 && abortMPU.CreatedBeforeDate != "") || (abortMPU.Days == 0 && abortMPU.CreatedBeforeDate == "") {
				return fmt.Errorf("invalid abort multipart upload lifecycle, must be set one of CreatedBeforeDate and Days")
			}
		}

		transitions := rule.Transitions
		if len(transitions) > 0 {
			if len(transitions) > 2 {
				return fmt.Errorf("invalid count of transition lifecycles, the count must than less than 3")
			}

			for _, transition := range transitions {
				if (transition.Days != 0 && transition.CreatedBeforeDate != "") || (transition.Days == 0 && transition.CreatedBeforeDate == "") {
					return fmt.Errorf("invalid transition lifecycle, must be set one of CreatedBeforeDate and Days")
				}
				if transition.StorageClass != StorageIA && transition.StorageClass != StorageArchive {
					return fmt.Errorf("invalid transition lifecylce, the value of storage class must be IA or Archive")
				}
			}
		} else if rule.Expiration == nil && abortMPU == nil && rule.NonVersionExpiration == nil && rule.NonVersionTransition == nil {
			return fmt.Errorf("invalid rule, must set one of Expiration, AbortMultipartUplaod, NonVersionExpiration, NonVersionTransition and Transitions")
		}
	}

	return nil
}

// GetBucketLifecycleResult defines GetBucketLifecycle's result object
type GetBucketLifecycleResult LifecycleConfiguration

// RefererXML defines Referer configuration
type RefererXML struct {
	XMLName           xml.Name `xml:"RefererConfiguration"`
	AllowEmptyReferer bool     `xml:"AllowEmptyReferer"`   // Allow empty referrer
	RefererList       []string `xml:"RefererList>Referer"` // Referer whitelist
}

// GetBucketRefererResult defines result object for GetBucketReferer request
type GetBucketRefererResult RefererXML

// LoggingXML defines logging configuration
type LoggingXML struct {
	XMLName        xml.Name       `xml:"BucketLoggingStatus"`
	LoggingEnabled LoggingEnabled `xml:"LoggingEnabled"` // The logging configuration information
}

type loggingXMLEmpty struct {
	XMLName xml.Name `xml:"BucketLoggingStatus"`
}

// LoggingEnabled defines the logging configuration information
type LoggingEnabled struct {
	XMLName      xml.Name `xml:"LoggingEnabled"`
	TargetBucket string   `xml:"TargetBucket"` // The bucket name for storing the log files
	TargetPrefix string   `xml:"TargetPrefix"` // The log file prefix
}

// GetBucketLoggingResult defines the result from GetBucketLogging request
type GetBucketLoggingResult LoggingXML

// WebsiteXML defines Website configuration
type WebsiteXML struct {
	XMLName       xml.Name      `xml:"WebsiteConfiguration"`
	IndexDocument IndexDocument `xml:"IndexDocument,omitempty"`            // The index page
	ErrorDocument ErrorDocument `xml:"ErrorDocument,omitempty"`            // The error page
	RoutingRules  []RoutingRule `xml:"RoutingRules>RoutingRule,omitempty"` // The routing Rule list
}

// IndexDocument defines the index page info
type IndexDocument struct {
	XMLName xml.Name `xml:"IndexDocument"`
	Suffix  string   `xml:"Suffix"` // The file name for the index page
}

// ErrorDocument defines the 404 error page info
type ErrorDocument struct {
	XMLName xml.Name `xml:"ErrorDocument"`
	Key     string   `xml:"Key"` // 404 error file name
}

// RoutingRule defines the routing rules
type RoutingRule struct {
	XMLName    xml.Name  `xml:"RoutingRule"`
	RuleNumber int       `xml:"RuleNumber,omitempty"` // The routing number
	Condition  Condition `xml:"Condition,omitempty"`  // The routing condition
	Redirect   Redirect  `xml:"Redirect,omitempty"`   // The routing redirect

}

// Condition defines codition in the RoutingRule
type Condition struct {
	XMLName                     xml.Name        `xml:"Condition"`
	KeyPrefixEquals             string          `xml:"KeyPrefixEquals,omitempty"`             // Matching objcet prefix
	HTTPErrorCodeReturnedEquals int             `xml:"HttpErrorCodeReturnedEquals,omitempty"` // The rule is for Accessing to the specified object
	IncludeHeader               []IncludeHeader `xml:"IncludeHeader"`                         // The rule is for request which include header
}

// IncludeHeader defines includeHeader in the RoutingRule's Condition
type IncludeHeader struct {
	XMLName xml.Name `xml:"IncludeHeader"`
	Key     string   `xml:"Key,omitempty"`    // The Include header key
	Equals  string   `xml:"Equals,omitempty"` // The Include header value
}

// Redirect defines redirect in the RoutingRule
type Redirect struct {
	XMLName               xml.Name      `xml:"Redirect"`
	RedirectType          string        `xml:"RedirectType,omitempty"`         // The redirect type, it have Mirror,External,Internal,AliCDN
	PassQueryString       *bool         `xml:"PassQueryString"`                // Whether to send the specified request's parameters, true or false
	MirrorURL             string        `xml:"MirrorURL,omitempty"`            // Mirror of the website address back to the source.
	MirrorPassQueryString *bool         `xml:"MirrorPassQueryString"`          // To Mirror of the website Whether to send the specified request's parameters, true or false
	MirrorFollowRedirect  *bool         `xml:"MirrorFollowRedirect"`           // Redirect the location, if the mirror return 3XX
	MirrorCheckMd5        *bool         `xml:"MirrorCheckMd5"`                 // Check the mirror is MD5.
	MirrorHeaders         MirrorHeaders `xml:"MirrorHeaders,omitempty"`        // Mirror headers
	Protocol              string        `xml:"Protocol,omitempty"`             // The redirect Protocol
	HostName              string        `xml:"HostName,omitempty"`             // The redirect HostName
	ReplaceKeyPrefixWith  string        `xml:"ReplaceKeyPrefixWith,omitempty"` // object name'Prefix replace the value
	HttpRedirectCode      int           `xml:"HttpRedirectCode,omitempty"`     // THe redirect http code
	ReplaceKeyWith        string        `xml:"ReplaceKeyWith,omitempty"`       // object name replace the value
}

// MirrorHeaders defines MirrorHeaders in the Redirect
type MirrorHeaders struct {
	XMLName xml.Name          `xml:"MirrorHeaders"`
	PassAll *bool             `xml:"PassAll"` // Penetrating all of headers to source website.
	Pass    []string          `xml:"Pass"`    // Penetrating some of headers to source website.
	Remove  []string          `xml:"Remove"`  // Prohibit passthrough some of headers to source website
	Set     []MirrorHeaderSet `xml:"Set"`     // Setting some of headers send to source website
}

// MirrorHeaderSet defines Set for Redirect's MirrorHeaders
type MirrorHeaderSet struct {
	XMLName xml.Name `xml:"Set"`
	Key     string   `xml:"Key,omitempty"`   // The mirror header key
	Value   string   `xml:"Value,omitempty"` // The mirror header value
}

// GetBucketWebsiteResult defines the result from GetBucketWebsite request.
type GetBucketWebsiteResult WebsiteXML

// CORSXML defines CORS configuration
type CORSXML struct {
	XMLName   xml.Name   `xml:"CORSConfiguration"`
	CORSRules []CORSRule `xml:"CORSRule"` // CORS rules
}

// CORSRule defines CORS rules
type CORSRule struct {
	XMLName       xml.Name `xml:"CORSRule"`
	AllowedOrigin []string `xml:"AllowedOrigin"` // Allowed origins. By default it's wildcard '*'
	AllowedMethod []string `xml:"AllowedMethod"` // Allowed methods
	AllowedHeader []string `xml:"AllowedHeader"` // Allowed headers
	ExposeHeader  []string `xml:"ExposeHeader"`  // Allowed response headers
	MaxAgeSeconds int      `xml:"MaxAgeSeconds"` // Max cache ages in seconds
}

// GetBucketCORSResult defines the result from GetBucketCORS request.
type GetBucketCORSResult CORSXML

// GetBucketInfoResult defines the result from GetBucketInfo request.
type GetBucketInfoResult struct {
	XMLName    xml.Name   `xml:"BucketInfo"`
	BucketInfo BucketInfo `xml:"Bucket"`
}

// BucketInfo defines Bucket information
type BucketInfo struct {
	XMLName          xml.Name  `xml:"Bucket"`
	Name             string    `xml:"Name"`                     // Bucket name
	Location         string    `xml:"Location"`                 // Bucket datacenter
	CreationDate     time.Time `xml:"CreationDate"`             // Bucket creation time
	ExtranetEndpoint string    `xml:"ExtranetEndpoint"`         // Bucket external endpoint
	IntranetEndpoint string    `xml:"IntranetEndpoint"`         // Bucket internal endpoint
	ACL              string    `xml:"AccessControlList>Grant"`  // Bucket ACL
	RedundancyType   string    `xml:"DataRedundancyType"`       // Bucket DataRedundancyType
	Owner            Owner     `xml:"Owner"`                    // Bucket owner
	StorageClass     string    `xml:"StorageClass"`             // Bucket storage class
	SseRule          SSERule   `xml:"ServerSideEncryptionRule"` // Bucket ServerSideEncryptionRule
	Versioning       string    `xml:"Versioning"`               // Bucket Versioning
}

type SSERule struct {
	XMLName        xml.Name `xml:"ServerSideEncryptionRule"` // Bucket ServerSideEncryptionRule
	KMSMasterKeyID string   `xml:"KMSMasterKeyID"`           // Bucket KMSMasterKeyID
	SSEAlgorithm   string   `xml:"SSEAlgorithm"`             // Bucket SSEAlgorithm
}

// ListObjectsResult defines the result from ListObjects request
type ListObjectsResult struct {
	XMLName        xml.Name           `xml:"ListBucketResult"`
	Prefix         string             `xml:"Prefix"`                // The object prefix
	Marker         string             `xml:"Marker"`                // The marker filter.
	MaxKeys        int                `xml:"MaxKeys"`               // Max keys to return
	Delimiter      string             `xml:"Delimiter"`             // The delimiter for grouping objects' name
	IsTruncated    bool               `xml:"IsTruncated"`           // Flag indicates if all results are returned (when it's false)
	NextMarker     string             `xml:"NextMarker"`            // The start point of the next query
	Objects        []ObjectProperties `xml:"Contents"`              // Object list
	CommonPrefixes []string           `xml:"CommonPrefixes>Prefix"` // You can think of commonprefixes as "folders" whose names end with the delimiter
}

// ObjectProperties defines Objecct properties
type ObjectProperties struct {
	XMLName      xml.Name  `xml:"Contents"`
	Key          string    `xml:"Key"`          // Object key
	Type         string    `xml:"Type"`         // Object type
	Size         int64     `xml:"Size"`         // Object size
	ETag         string    `xml:"ETag"`         // Object ETag
	Owner        Owner     `xml:"Owner"`        // Object owner information
	LastModified time.Time `xml:"LastModified"` // Object last modified time
	StorageClass string    `xml:"StorageClass"` // Object storage class (Standard, IA, Archive)
}

// ListObjectVersionsResult defines the result from ListObjectVersions request
type ListObjectVersionsResult struct {
	XMLName             xml.Name                       `xml:"ListVersionsResult"`
	Name                string                         `xml:"Name"`                  // The Bucket Name
	Owner               Owner                          `xml:"Owner"`                 // The owner of bucket
	Prefix              string                         `xml:"Prefix"`                // The object prefix
	KeyMarker           string                         `xml:"KeyMarker"`             // The start marker filter.
	VersionIdMarker     string                         `xml:"VersionIdMarker"`       // The start VersionIdMarker filter.
	MaxKeys             int                            `xml:"MaxKeys"`               // Max keys to return
	Delimiter           string                         `xml:"Delimiter"`             // The delimiter for grouping objects' name
	IsTruncated         bool                           `xml:"IsTruncated"`           // Flag indicates if all results are returned (when it's false)
	NextKeyMarker       string                         `xml:"NextKeyMarker"`         // The start point of the next query
	NextVersionIdMarker string                         `xml:"NextVersionIdMarker"`   // The start point of the next query
	CommonPrefixes      []string                       `xml:"CommonPrefixes>Prefix"` // You can think of commonprefixes as "folders" whose names end with the delimiter
	ObjectDeleteMarkers []ObjectDeleteMarkerProperties `xml:"DeleteMarker"`          // DeleteMarker list
	ObjectVersions      []ObjectVersionProperties      `xml:"Version"`               // version list
}

type ObjectDeleteMarkerProperties struct {
	XMLName      xml.Name  `xml:"DeleteMarker"`
	Key          string    `xml:"Key"`          // The Object Key
	VersionId    string    `xml:"VersionId"`    // The Object VersionId
	IsLatest     bool      `xml:"IsLatest"`     // is current version or not
	LastModified time.Time `xml:"LastModified"` // Object last modified time
	Owner        Owner     `xml:"Owner"`        // bucket owner element
}

type ObjectVersionProperties struct {
	XMLName      xml.Name  `xml:"Version"`
	Key          string    `xml:"Key"`          // The Object Key
	VersionId    string    `xml:"VersionId"`    // The Object VersionId
	IsLatest     bool      `xml:"IsLatest"`     // is latest version or not
	LastModified time.Time `xml:"LastModified"` // Object last modified time
	Type         string    `xml:"Type"`         // Object type
	Size         int64     `xml:"Size"`         // Object size
	ETag         string    `xml:"ETag"`         // Object ETag
	StorageClass string    `xml:"StorageClass"` // Object storage class (Standard, IA, Archive)
	Owner        Owner     `xml:"Owner"`        // bucket owner element
}

// Owner defines Bucket/Object's owner
type Owner struct {
	XMLName     xml.Name `xml:"Owner"`
	ID          string   `xml:"ID"`          // Owner ID
	DisplayName string   `xml:"DisplayName"` // Owner's display name
}

// CopyObjectResult defines result object of CopyObject
type CopyObjectResult struct {
	XMLName      xml.Name  `xml:"CopyObjectResult"`
	LastModified time.Time `xml:"LastModified"` // New object's last modified time.
	ETag         string    `xml:"ETag"`         // New object's ETag
}

// GetObjectACLResult defines result of GetObjectACL request
type GetObjectACLResult GetBucketACLResult

type deleteXML struct {
	XMLName xml.Name       `xml:"Delete"`
	Objects []DeleteObject `xml:"Object"` // Objects to delete
	Quiet   bool           `xml:"Quiet"`  // Flag of quiet mode.
}

// DeleteObject defines the struct for deleting object
type DeleteObject struct {
	XMLName   xml.Name `xml:"Object"`
	Key       string   `xml:"Key"`                 // Object name
	VersionId string   `xml:"VersionId,omitempty"` // Object VersionId
}

// DeleteObjectsResult defines result of DeleteObjects request
type DeleteObjectsResult struct {
	XMLName        xml.Name
	DeletedObjects []string // Deleted object key list
}

// DeleteObjectsResult_inner defines result of DeleteObjects request
type DeleteObjectVersionsResult struct {
	XMLName              xml.Name         `xml:"DeleteResult"`
	DeletedObjectsDetail []DeletedKeyInfo `xml:"Deleted"` // Deleted object detail info
}

// DeleteKeyInfo defines object delete info
type DeletedKeyInfo struct {
	XMLName               xml.Name `xml:"Deleted"`
	Key                   string   `xml:"Key"`                   // Object key
	VersionId             string   `xml:"VersionId"`             // VersionId
	DeleteMarker          bool     `xml:"DeleteMarker"`          // Object DeleteMarker
	DeleteMarkerVersionId string   `xml:"DeleteMarkerVersionId"` // Object DeleteMarkerVersionId
}

// InitiateMultipartUploadResult defines result of InitiateMultipartUpload request
type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`   // Bucket name
	Key      string   `xml:"Key"`      // Object name to upload
	UploadID string   `xml:"UploadId"` // Generated UploadId
}

// UploadPart defines the upload/copy part
type UploadPart struct {
	XMLName    xml.Name `xml:"Part"`
	PartNumber int      `xml:"PartNumber"` // Part number
	ETag       string   `xml:"ETag"`       // ETag value of the part's data
}

type uploadParts []UploadPart

func (slice uploadParts) Len() int {
	return len(slice)
}

func (slice uploadParts) Less(i, j int) bool {
	return slice[i].PartNumber < slice[j].PartNumber
}

func (slice uploadParts) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

// UploadPartCopyResult defines result object of multipart copy request.
type UploadPartCopyResult struct {
	XMLName      xml.Name  `xml:"CopyPartResult"`
	LastModified time.Time `xml:"LastModified"` // Last modified time
	ETag         string    `xml:"ETag"`         // ETag
}

type completeMultipartUploadXML struct {
	XMLName xml.Name     `xml:"CompleteMultipartUpload"`
	Part    []UploadPart `xml:"Part"`
}

// CompleteMultipartUploadResult defines result object of CompleteMultipartUploadRequest
type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"` // Object URL
	Bucket   string   `xml:"Bucket"`   // Bucket name
	ETag     string   `xml:"ETag"`     // Object ETag
	Key      string   `xml:"Key"`      // Object name
}

// ListUploadedPartsResult defines result object of ListUploadedParts
type ListUploadedPartsResult struct {
	XMLName              xml.Name       `xml:"ListPartsResult"`
	Bucket               string         `xml:"Bucket"`               // Bucket name
	Key                  string         `xml:"Key"`                  // Object name
	UploadID             string         `xml:"UploadId"`             // Upload ID
	NextPartNumberMarker string         `xml:"NextPartNumberMarker"` // Next part number
	MaxParts             int            `xml:"MaxParts"`             // Max parts count
	IsTruncated          bool           `xml:"IsTruncated"`          // Flag indicates all entries returned.false: all entries returned.
	UploadedParts        []UploadedPart `xml:"Part"`                 // Uploaded parts
}

// UploadedPart defines uploaded part
type UploadedPart struct {
	XMLName      xml.Name  `xml:"Part"`
	PartNumber   int       `xml:"PartNumber"`   // Part number
	LastModified time.Time `xml:"LastModified"` // Last modified time
	ETag         string    `xml:"ETag"`         // ETag cache
	Size         int       `xml:"Size"`         // Part size
}

// ListMultipartUploadResult defines result object of ListMultipartUpload
type ListMultipartUploadResult struct {
	XMLName            xml.Name            `xml:"ListMultipartUploadsResult"`
	Bucket             string              `xml:"Bucket"`                // Bucket name
	Delimiter          string              `xml:"Delimiter"`             // Delimiter for grouping object.
	Prefix             string              `xml:"Prefix"`                // Object prefix
	KeyMarker          string              `xml:"KeyMarker"`             // Object key marker
	UploadIDMarker     string              `xml:"UploadIdMarker"`        // UploadId marker
	NextKeyMarker      string              `xml:"NextKeyMarker"`         // Next key marker, if not all entries returned.
	NextUploadIDMarker string              `xml:"NextUploadIdMarker"`    // Next uploadId marker, if not all entries returned.
	MaxUploads         int                 `xml:"MaxUploads"`            // Max uploads to return
	IsTruncated        bool                `xml:"IsTruncated"`           // Flag indicates all entries are returned.
	Uploads            []UncompletedUpload `xml:"Upload"`                // Ongoing uploads (not completed, not aborted)
	CommonPrefixes     []string            `xml:"CommonPrefixes>Prefix"` // Common prefixes list.
}

// UncompletedUpload structure wraps an uncompleted upload task
type UncompletedUpload struct {
	XMLName   xml.Name  `xml:"Upload"`
	Key       string    `xml:"Key"`       // Object name
	UploadID  string    `xml:"UploadId"`  // The UploadId
	Initiated time.Time `xml:"Initiated"` // Initialization time in the format such as 2012-02-23T04:18:23.000Z
}

// ProcessObjectResult defines result object of ProcessObject
type ProcessObjectResult struct {
	Bucket   string `json:"bucket"`
	FileSize int    `json:"fileSize"`
	Object   string `json:"object"`
	Status   string `json:"status"`
}

// decodeDeleteObjectsResult decodes deleting objects result in URL encoding
func decodeDeleteObjectsResult(result *DeleteObjectVersionsResult) error {
	var err error
	for i := 0; i < len(result.DeletedObjectsDetail); i++ {
		result.DeletedObjectsDetail[i].Key, err = url.QueryUnescape(result.DeletedObjectsDetail[i].Key)
		if err != nil {
			return err
		}
	}
	return nil
}

// decodeListObjectsResult decodes list objects result in URL encoding
func decodeListObjectsResult(result *ListObjectsResult) error {
	var err error
	result.Prefix, err = url.QueryUnescape(result.Prefix)
	if err != nil {
		return err
	}
	result.Marker, err = url.QueryUnescape(result.Marker)
	if err != nil {
		return err
	}
	result.Delimiter, err = url.QueryUnescape(result.Delimiter)
	if err != nil {
		return err
	}
	result.NextMarker, err = url.QueryUnescape(result.NextMarker)
	if err != nil {
		return err
	}
	for i := 0; i < len(result.Objects); i++ {
		result.Objects[i].Key, err = url.QueryUnescape(result.Objects[i].Key)
		if err != nil {
			return err
		}
	}
	for i := 0; i < len(result.CommonPrefixes); i++ {
		result.CommonPrefixes[i], err = url.QueryUnescape(result.CommonPrefixes[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// decodeListObjectVersionsResult decodes list version objects result in URL encoding
func decodeListObjectVersionsResult(result *ListObjectVersionsResult) error {
	var err error

	// decode:Delimiter
	result.Delimiter, err = url.QueryUnescape(result.Delimiter)
	if err != nil {
		return err
	}

	// decode Prefix
	result.Prefix, err = url.QueryUnescape(result.Prefix)
	if err != nil {
		return err
	}

	// decode KeyMarker
	result.KeyMarker, err = url.QueryUnescape(result.KeyMarker)
	if err != nil {
		return err
	}

	// decode VersionIdMarker
	result.VersionIdMarker, err = url.QueryUnescape(result.VersionIdMarker)
	if err != nil {
		return err
	}

	// decode NextKeyMarker
	result.NextKeyMarker, err = url.QueryUnescape(result.NextKeyMarker)
	if err != nil {
		return err
	}

	// decode NextVersionIdMarker
	result.NextVersionIdMarker, err = url.QueryUnescape(result.NextVersionIdMarker)
	if err != nil {
		return err
	}

	// decode CommonPrefixes
	for i := 0; i < len(result.CommonPrefixes); i++ {
		result.CommonPrefixes[i], err = url.QueryUnescape(result.CommonPrefixes[i])
		if err != nil {
			return err
		}
	}

	// decode deleteMarker
	for i := 0; i < len(result.ObjectDeleteMarkers); i++ {
		result.ObjectDeleteMarkers[i].Key, err = url.QueryUnescape(result.ObjectDeleteMarkers[i].Key)
		if err != nil {
			return err
		}
	}

	// decode ObjectVersions
	for i := 0; i < len(result.ObjectVersions); i++ {
		result.ObjectVersions[i].Key, err = url.QueryUnescape(result.ObjectVersions[i].Key)
		if err != nil {
			return err
		}
	}

	return nil
}

// decodeListUploadedPartsResult decodes
func decodeListUploadedPartsResult(result *ListUploadedPartsResult) error {
	var err error
	result.Key, err = url.QueryUnescape(result.Key)
	if err != nil {
		return err
	}
	return nil
}

// decodeListMultipartUploadResult decodes list multipart upload result in URL encoding
func decodeListMultipartUploadResult(result *ListMultipartUploadResult) error {
	var err error
	result.Prefix, err = url.QueryUnescape(result.Prefix)
	if err != nil {
		return err
	}
	result.Delimiter, err = url.QueryUnescape(result.Delimiter)
	if err != nil {
		return err
	}
	result.KeyMarker, err = url.QueryUnescape(result.KeyMarker)
	if err != nil {
		return err
	}
	result.NextKeyMarker, err = url.QueryUnescape(result.NextKeyMarker)
	if err != nil {
		return err
	}
	for i := 0; i < len(result.Uploads); i++ {
		result.Uploads[i].Key, err = url.QueryUnescape(result.Uploads[i].Key)
		if err != nil {
			return err
		}
	}
	for i := 0; i < len(result.CommonPrefixes); i++ {
		result.CommonPrefixes[i], err = url.QueryUnescape(result.CommonPrefixes[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// createBucketConfiguration defines the configuration for creating a bucket.
type createBucketConfiguration struct {
	XMLName            xml.Name           `xml:"CreateBucketConfiguration"`
	StorageClass       StorageClassType   `xml:"StorageClass,omitempty"`
	DataRedundancyType DataRedundancyType `xml:"DataRedundancyType,omitempty"`
}

// LiveChannelConfiguration defines the configuration for live-channel
type LiveChannelConfiguration struct {
	XMLName     xml.Name          `xml:"LiveChannelConfiguration"`
	Description string            `xml:"Description,omitempty"` //Description of live-channel, up to 128 bytes
	Status      string            `xml:"Status,omitempty"`      //Specify the status of livechannel
	Target      LiveChannelTarget `xml:"Target"`                //target configuration of live-channel
	// use point instead of struct to avoid omit empty snapshot
	Snapshot *LiveChannelSnapshot `xml:"Snapshot,omitempty"` //snapshot configuration of live-channel
}

// LiveChannelTarget target configuration of live-channel
type LiveChannelTarget struct {
	XMLName      xml.Name `xml:"Target"`
	Type         string   `xml:"Type"`                   //the type of object, only supports HLS
	FragDuration int      `xml:"FragDuration,omitempty"` //the length of each ts object (in seconds), in the range [1,100]
	FragCount    int      `xml:"FragCount,omitempty"`    //the number of ts objects in the m3u8 object, in the range of [1,100]
	PlaylistName string   `xml:"PlaylistName,omitempty"` //the name of m3u8 object, which must end with ".m3u8" and the length range is [6,128]
}

// LiveChannelSnapshot snapshot configuration of live-channel
type LiveChannelSnapshot struct {
	XMLName     xml.Name `xml:"Snapshot"`
	RoleName    string   `xml:"RoleName,omitempty"`    //The role of snapshot operations, it sholud has write permission of DestBucket and the permission to send messages to the NotifyTopic.
	DestBucket  string   `xml:"DestBucket,omitempty"`  //Bucket the snapshots will be written to. should be the same owner as the source bucket.
	NotifyTopic string   `xml:"NotifyTopic,omitempty"` //Topics of MNS for notifying users of high frequency screenshot operation results
	Interval    int      `xml:"Interval,omitempty"`    //interval of snapshots, threre is no snapshot if no I-frame during the interval time
}

// CreateLiveChannelResult the result of crete live-channel
type CreateLiveChannelResult struct {
	XMLName     xml.Name `xml:"CreateLiveChannelResult"`
	PublishUrls []string `xml:"PublishUrls>Url"` //push urls list
	PlayUrls    []string `xml:"PlayUrls>Url"`    //play urls list
}

// LiveChannelStat the result of get live-channel state
type LiveChannelStat struct {
	XMLName       xml.Name         `xml:"LiveChannelStat"`
	Status        string           `xml:"Status"`        //Current push status of live-channel: Disabled,Live,Idle
	ConnectedTime time.Time        `xml:"ConnectedTime"` //The time when the client starts pushing, format: ISO8601
	RemoteAddr    string           `xml:"RemoteAddr"`    //The ip address of the client
	Video         LiveChannelVideo `xml:"Video"`         //Video stream information
	Audio         LiveChannelAudio `xml:"Audio"`         //Audio stream information
}

// LiveChannelVideo video stream information
type LiveChannelVideo struct {
	XMLName   xml.Name `xml:"Video"`
	Width     int      `xml:"Width"`     //Width (unit: pixels)
	Height    int      `xml:"Height"`    //Height (unit: pixels)
	FrameRate int      `xml:"FrameRate"` //FramRate
	Bandwidth int      `xml:"Bandwidth"` //Bandwidth (unit: B/s)
}

// LiveChannelAudio audio stream information
type LiveChannelAudio struct {
	XMLName    xml.Name `xml:"Audio"`
	SampleRate int      `xml:"SampleRate"` //SampleRate
	Bandwidth  int      `xml:"Bandwidth"`  //Bandwidth (unit: B/s)
	Codec      string   `xml:"Codec"`      //Encoding forma
}

// LiveChannelHistory the result of GetLiveChannelHistory, at most return up to lastest 10 push records
type LiveChannelHistory struct {
	XMLName xml.Name     `xml:"LiveChannelHistory"`
	Record  []LiveRecord `xml:"LiveRecord"` //push records list
}

// LiveRecord push recode
type LiveRecord struct {
	XMLName    xml.Name  `xml:"LiveRecord"`
	StartTime  time.Time `xml:"StartTime"`  //StartTime, format: ISO8601
	EndTime    time.Time `xml:"EndTime"`    //EndTime, format: ISO8601
	RemoteAddr string    `xml:"RemoteAddr"` //The ip address of remote client
}

// ListLiveChannelResult the result of ListLiveChannel
type ListLiveChannelResult struct {
	XMLName     xml.Name          `xml:"ListLiveChannelResult"`
	Prefix      string            `xml:"Prefix"`      //Filter by the name start with the value of "Prefix"
	Marker      string            `xml:"Marker"`      //cursor from which starting list
	MaxKeys     int               `xml:"MaxKeys"`     //The maximum count returned. the default value is 100. it cannot be greater than 1000.
	IsTruncated bool              `xml:"IsTruncated"` //Indicates whether all results have been returned, "true" indicates partial results returned while "false" indicates all results have been returned
	NextMarker  string            `xml:"NextMarker"`  //NextMarker indicate the Marker value of the next request
	LiveChannel []LiveChannelInfo `xml:"LiveChannel"` //The infomation of live-channel
}

// LiveChannelInfo the infomation of live-channel
type LiveChannelInfo struct {
	XMLName      xml.Name  `xml:"LiveChannel"`
	Name         string    `xml:"Name"`            //The name of live-channel
	Description  string    `xml:"Description"`     //Description of live-channel
	Status       string    `xml:"Status"`          //Status: disabled or enabled
	LastModified time.Time `xml:"LastModified"`    //Last modification time, format: ISO8601
	PublishUrls  []string  `xml:"PublishUrls>Url"` //push urls list
	PlayUrls     []string  `xml:"PlayUrls>Url"`    //play urls list
}

// Tag a tag for the object
type Tag struct {
	XMLName xml.Name `xml:"Tag"`
	Key     string   `xml:"Key"`
	Value   string   `xml:"Value"`
}

// Tagging tagset for the object
type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	Tags    []Tag    `xml:"TagSet>Tag,omitempty"`
}

// for GetObjectTagging return value
type GetObjectTaggingResult Tagging

// VersioningConfig for the bucket
type VersioningConfig struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	Status  string   `xml:"Status"`
}

type GetBucketVersioningResult VersioningConfig

// Server Encryption rule for the bucket
type ServerEncryptionRule struct {
	XMLName    xml.Name       `xml:"ServerSideEncryptionRule"`
	SSEDefault SSEDefaultRule `xml:"ApplyServerSideEncryptionByDefault"`
}

// Server Encryption deafult rule for the bucket
type SSEDefaultRule struct {
	XMLName        xml.Name `xml:"ApplyServerSideEncryptionByDefault"`
	SSEAlgorithm   string   `xml:"SSEAlgorithm"`
	KMSMasterKeyID string   `xml:"KMSMasterKeyID"`
}

type GetBucketEncryptionResult ServerEncryptionRule
type GetBucketTaggingResult Tagging

type BucketStat struct {
	XMLName              xml.Name `xml:"BucketStat"`
	Storage              int64    `xml:"Storage"`
	ObjectCount          int64    `xml:"ObjectCount"`
	MultipartUploadCount int64    `xml:"MultipartUploadCount"`
}
type GetBucketStatResult BucketStat

// RequestPaymentConfiguration define the request payment configuration
type RequestPaymentConfiguration struct {
	XMLName xml.Name `xml:"RequestPaymentConfiguration"`
	Payer   string   `xml:"Payer,omitempty"`
}

// BucketQoSConfiguration define QoS configuration
type BucketQoSConfiguration struct {
	XMLName                   xml.Name `xml:"QoSConfiguration"`
	TotalUploadBandwidth      *int     `xml:"TotalUploadBandwidth"`      // Total upload bandwidth
	IntranetUploadBandwidth   *int     `xml:"IntranetUploadBandwidth"`   // Intranet upload bandwidth
	ExtranetUploadBandwidth   *int     `xml:"ExtranetUploadBandwidth"`   // Extranet upload bandwidth
	TotalDownloadBandwidth    *int     `xml:"TotalDownloadBandwidth"`    // Total download bandwidth
	IntranetDownloadBandwidth *int     `xml:"IntranetDownloadBandwidth"` // Intranet download bandwidth
	ExtranetDownloadBandwidth *int     `xml:"ExtranetDownloadBandwidth"` // Extranet download bandwidth
	TotalQPS                  *int     `xml:"TotalQps"`                  // Total Qps
	IntranetQPS               *int     `xml:"IntranetQps"`               // Intranet Qps
	ExtranetQPS               *int     `xml:"ExtranetQps"`               // Extranet Qps
}

// UserQoSConfiguration define QoS and Range configuration
type UserQoSConfiguration struct {
	XMLName xml.Name `xml:"QoSConfiguration"`
	Region  string   `xml:"Region,omitempty"` // Effective area of Qos configuration
	BucketQoSConfiguration
}

//////////////////////////////////////////////////////////////
/////////////////// Select OBject ////////////////////////////
//////////////////////////////////////////////////////////////

type CsvMetaRequest struct {
	XMLName            xml.Name           `xml:"CsvMetaRequest"`
	InputSerialization InputSerialization `xml:"InputSerialization"`
	OverwriteIfExists  *bool              `xml:"OverwriteIfExists,omitempty"`
}

// encodeBase64 encode base64 of the CreateSelectObjectMeta api request params
func (meta *CsvMetaRequest) encodeBase64() {
	meta.InputSerialization.CSV.RecordDelimiter =
		base64.StdEncoding.EncodeToString([]byte(meta.InputSerialization.CSV.RecordDelimiter))
	meta.InputSerialization.CSV.FieldDelimiter =
		base64.StdEncoding.EncodeToString([]byte(meta.InputSerialization.CSV.FieldDelimiter))
	meta.InputSerialization.CSV.QuoteCharacter =
		base64.StdEncoding.EncodeToString([]byte(meta.InputSerialization.CSV.QuoteCharacter))
}

type JsonMetaRequest struct {
	XMLName            xml.Name           `xml:"JsonMetaRequest"`
	InputSerialization InputSerialization `xml:"InputSerialization"`
	OverwriteIfExists  *bool              `xml:"OverwriteIfExists,omitempty"`
}

type InputSerialization struct {
	XMLName         xml.Name `xml:"InputSerialization"`
	CSV             CSV      `xml:CSV,omitempty`
	JSON            JSON     `xml:JSON,omitempty`
	CompressionType string   `xml:"CompressionType,omitempty"`
}
type CSV struct {
	XMLName         xml.Name `xml:"CSV"`
	RecordDelimiter string   `xml:"RecordDelimiter,omitempty"`
	FieldDelimiter  string   `xml:"FieldDelimiter,omitempty"`
	QuoteCharacter  string   `xml:"QuoteCharacter,omitempty"`
}

type JSON struct {
	XMLName  xml.Name `xml:"JSON"`
	JSONType string   `xml:"Type,omitempty"`
}

// SelectRequest is for the SelectObject request params of json file
type SelectRequest struct {
	XMLName                   xml.Name                  `xml:"SelectRequest"`
	Expression                string                    `xml:"Expression"`
	InputSerializationSelect  InputSerializationSelect  `xml:"InputSerialization"`
	OutputSerializationSelect OutputSerializationSelect `xml:"OutputSerialization"`
	SelectOptions             SelectOptions             `xml:"Options,omitempty"`
}
type InputSerializationSelect struct {
	XMLName         xml.Name        `xml:"InputSerialization"`
	CsvBodyInput    CSVSelectInput  `xml:CSV,omitempty`
	JsonBodyInput   JSONSelectInput `xml:JSON,omitempty`
	CompressionType string          `xml:"CompressionType,omitempty"`
}
type CSVSelectInput struct {
	XMLName          xml.Name `xml:"CSV"`
	FileHeaderInfo   string   `xml:"FileHeaderInfo,omitempty"`
	RecordDelimiter  string   `xml:"RecordDelimiter,omitempty"`
	FieldDelimiter   string   `xml:"FieldDelimiter,omitempty"`
	QuoteCharacter   string   `xml:"QuoteCharacter,omitempty"`
	CommentCharacter string   `xml:"CommentCharacter,omitempty"`
	Range            string   `xml:"Range,omitempty"`
	SplitRange       string
}
type JSONSelectInput struct {
	XMLName                 xml.Name `xml:"JSON"`
	JSONType                string   `xml:"Type,omitempty"`
	Range                   string   `xml:"Range,omitempty"`
	ParseJSONNumberAsString *bool    `xml:"ParseJsonNumberAsString"`
	SplitRange              string
}

func (jsonInput *JSONSelectInput) JsonIsEmpty() bool {
	if jsonInput.JSONType != "" {
		return false
	}
	return true
}

type OutputSerializationSelect struct {
	XMLName          xml.Name         `xml:"OutputSerialization"`
	CsvBodyOutput    CSVSelectOutput  `xml:CSV,omitempty`
	JsonBodyOutput   JSONSelectOutput `xml:JSON,omitempty`
	OutputRawData    *bool            `xml:"OutputRawData,omitempty"`
	KeepAllColumns   *bool            `xml:"KeepAllColumns,omitempty"`
	EnablePayloadCrc *bool            `xml:"EnablePayloadCrc,omitempty"`
	OutputHeader     *bool            `xml:"OutputHeader,omitempty"`
}
type CSVSelectOutput struct {
	XMLName         xml.Name `xml:"CSV"`
	RecordDelimiter string   `xml:"RecordDelimiter,omitempty"`
	FieldDelimiter  string   `xml:"FieldDelimiter,omitempty"`
}
type JSONSelectOutput struct {
	XMLName         xml.Name `xml:"JSON"`
	RecordDelimiter string   `xml:"RecordDelimiter,omitempty"`
}

func (selectReq *SelectRequest) encodeBase64() {
	if selectReq.InputSerializationSelect.JsonBodyInput.JsonIsEmpty() {
		selectReq.csvEncodeBase64()
	} else {
		selectReq.jsonEncodeBase64()
	}
}

// csvEncodeBase64 encode base64 of the SelectObject api request params
func (selectReq *SelectRequest) csvEncodeBase64() {
	selectReq.Expression = base64.StdEncoding.EncodeToString([]byte(selectReq.Expression))
	selectReq.InputSerializationSelect.CsvBodyInput.RecordDelimiter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.InputSerializationSelect.CsvBodyInput.RecordDelimiter))
	selectReq.InputSerializationSelect.CsvBodyInput.FieldDelimiter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.InputSerializationSelect.CsvBodyInput.FieldDelimiter))
	selectReq.InputSerializationSelect.CsvBodyInput.QuoteCharacter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.InputSerializationSelect.CsvBodyInput.QuoteCharacter))
	selectReq.InputSerializationSelect.CsvBodyInput.CommentCharacter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.InputSerializationSelect.CsvBodyInput.CommentCharacter))
	selectReq.OutputSerializationSelect.CsvBodyOutput.FieldDelimiter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.OutputSerializationSelect.CsvBodyOutput.FieldDelimiter))
	selectReq.OutputSerializationSelect.CsvBodyOutput.RecordDelimiter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.OutputSerializationSelect.CsvBodyOutput.RecordDelimiter))

	// handle Range
	if selectReq.InputSerializationSelect.CsvBodyInput.Range != "" {
		selectReq.InputSerializationSelect.CsvBodyInput.Range = "line-range=" + selectReq.InputSerializationSelect.CsvBodyInput.Range
	}

	if selectReq.InputSerializationSelect.CsvBodyInput.SplitRange != "" {
		selectReq.InputSerializationSelect.CsvBodyInput.Range = "split-range=" + selectReq.InputSerializationSelect.CsvBodyInput.SplitRange
	}
}

// jsonEncodeBase64 encode base64 of the SelectObject api request params
func (selectReq *SelectRequest) jsonEncodeBase64() {
	selectReq.Expression = base64.StdEncoding.EncodeToString([]byte(selectReq.Expression))
	selectReq.OutputSerializationSelect.JsonBodyOutput.RecordDelimiter =
		base64.StdEncoding.EncodeToString([]byte(selectReq.OutputSerializationSelect.JsonBodyOutput.RecordDelimiter))

	// handle Range
	if selectReq.InputSerializationSelect.JsonBodyInput.Range != "" {
		selectReq.InputSerializationSelect.JsonBodyInput.Range = "line-range=" + selectReq.InputSerializationSelect.JsonBodyInput.Range
	}

	if selectReq.InputSerializationSelect.JsonBodyInput.SplitRange != "" {
		selectReq.InputSerializationSelect.JsonBodyInput.Range = "split-range=" + selectReq.InputSerializationSelect.JsonBodyInput.SplitRange
	}
}

// CsvOptions is a element in the SelectObject api request's params
type SelectOptions struct {
	XMLName                  xml.Name `xml:"Options"`
	SkipPartialDataRecord    *bool    `xml:"SkipPartialDataRecord,omitempty"`
	MaxSkippedRecordsAllowed string   `xml:"MaxSkippedRecordsAllowed,omitempty"`
}

// SelectObjectResult is the SelectObject api's return
type SelectObjectResult struct {
	Version          byte
	FrameType        int32
	PayloadLength    int32
	HeaderCheckSum   uint32
	Offset           uint64
	Data             string           // DataFrame
	EndFrame         EndFrame         // EndFrame
	MetaEndFrameCSV  MetaEndFrameCSV  // MetaEndFrameCSV
	MetaEndFrameJSON MetaEndFrameJSON // MetaEndFrameJSON
	PayloadChecksum  uint32
	ReadFlagInfo
}

// ReadFlagInfo if reading the frame data, recode the reading status
type ReadFlagInfo struct {
	OpenLine            bool
	ConsumedBytesLength int32
	EnablePayloadCrc    bool
	OutputRawData       bool
}

// EndFrame is EndFrameType of SelectObject api
type EndFrame struct {
	TotalScanned   int64
	HTTPStatusCode int32
	ErrorMsg       string
}

// MetaEndFrameCSV is MetaEndFrameCSVType of CreateSelectObjectMeta
type MetaEndFrameCSV struct {
	TotalScanned int64
	Status       int32
	SplitsCount  int32
	RowsCount    int64
	ColumnsCount int32
	ErrorMsg     string
}

// MetaEndFrameJSON is MetaEndFrameJSON of CreateSelectObjectMeta
type MetaEndFrameJSON struct {
	TotalScanned int64
	Status       int32
	SplitsCount  int32
	RowsCount    int64
	ErrorMsg     string
}
