package oss

import "os"

// ACLType bucket/object ACL
type ACLType string

const (
	// ACLPrivate definition : private read and write
	ACLPrivate ACLType = "private"

	// ACLPublicRead definition : public read and private write
	ACLPublicRead ACLType = "public-read"

	// ACLPublicReadWrite definition : public read and public write
	ACLPublicReadWrite ACLType = "public-read-write"

	// ACLDefault Object. It's only applicable for object.
	ACLDefault ACLType = "default"
)

// bucket versioning status
type VersioningStatus string

const (
	// Versioning Status definition: Enabled
	VersionEnabled VersioningStatus = "Enabled"

	// Versioning Status definition: Suspended
	VersionSuspended VersioningStatus = "Suspended"
)

// MetadataDirectiveType specifying whether use the metadata of source object when copying object.
type MetadataDirectiveType string

const (
	// MetaCopy the target object's metadata is copied from the source one
	MetaCopy MetadataDirectiveType = "COPY"

	// MetaReplace the target object's metadata is created as part of the copy request (not same as the source one)
	MetaReplace MetadataDirectiveType = "REPLACE"
)

// TaggingDirectiveType specifying whether use the tagging of source object when copying object.
type TaggingDirectiveType string

const (
	// TaggingCopy the target object's tagging is copied from the source one
	TaggingCopy TaggingDirectiveType = "COPY"

	// TaggingReplace the target object's tagging is created as part of the copy request (not same as the source one)
	TaggingReplace TaggingDirectiveType = "REPLACE"
)

// AlgorithmType specifying the server side encryption algorithm name
type AlgorithmType string

const (
	KMSAlgorithm AlgorithmType = "KMS"
	AESAlgorithm AlgorithmType = "AES256"
)

// StorageClassType bucket storage type
type StorageClassType string

const (
	// StorageStandard standard
	StorageStandard StorageClassType = "Standard"

	// StorageIA infrequent access
	StorageIA StorageClassType = "IA"

	// StorageArchive archive
	StorageArchive StorageClassType = "Archive"
)

// RedundancyType bucket data Redundancy type
type DataRedundancyType string

const (
	// RedundancyLRS Local redundancy, default value
	RedundancyLRS DataRedundancyType = "LRS"

	// RedundancyZRS Same city redundancy
	RedundancyZRS DataRedundancyType = "ZRS"
)

// PayerType the type of request payer
type PayerType string

const (
	// Requester the requester who send the request
	Requester PayerType = "Requester"

	// BucketOwner the requester who send the request
	BucketOwner PayerType = "BucketOwner"
)

// HTTPMethod HTTP request method
type HTTPMethod string

const (
	// HTTPGet HTTP GET
	HTTPGet HTTPMethod = "GET"

	// HTTPPut HTTP PUT
	HTTPPut HTTPMethod = "PUT"

	// HTTPHead HTTP HEAD
	HTTPHead HTTPMethod = "HEAD"

	// HTTPPost HTTP POST
	HTTPPost HTTPMethod = "POST"

	// HTTPDelete HTTP DELETE
	HTTPDelete HTTPMethod = "DELETE"
)

// HTTP headers
const (
	HTTPHeaderAcceptEncoding     string = "Accept-Encoding"
	HTTPHeaderAuthorization             = "Authorization"
	HTTPHeaderCacheControl              = "Cache-Control"
	HTTPHeaderContentDisposition        = "Content-Disposition"
	HTTPHeaderContentEncoding           = "Content-Encoding"
	HTTPHeaderContentLength             = "Content-Length"
	HTTPHeaderContentMD5                = "Content-MD5"
	HTTPHeaderContentType               = "Content-Type"
	HTTPHeaderContentLanguage           = "Content-Language"
	HTTPHeaderDate                      = "Date"
	HTTPHeaderEtag                      = "ETag"
	HTTPHeaderExpires                   = "Expires"
	HTTPHeaderHost                      = "Host"
	HTTPHeaderLastModified              = "Last-Modified"
	HTTPHeaderRange                     = "Range"
	HTTPHeaderLocation                  = "Location"
	HTTPHeaderOrigin                    = "Origin"
	HTTPHeaderServer                    = "Server"
	HTTPHeaderUserAgent                 = "User-Agent"
	HTTPHeaderIfModifiedSince           = "If-Modified-Since"
	HTTPHeaderIfUnmodifiedSince         = "If-Unmodified-Since"
	HTTPHeaderIfMatch                   = "If-Match"
	HTTPHeaderIfNoneMatch               = "If-None-Match"
	HTTPHeaderACReqMethod               = "Access-Control-Request-Method"
	HTTPHeaderACReqHeaders              = "Access-Control-Request-Headers"

	HTTPHeaderOssACL                         = "X-Oss-Acl"
	HTTPHeaderOssMetaPrefix                  = "X-Oss-Meta-"
	HTTPHeaderOssObjectACL                   = "X-Oss-Object-Acl"
	HTTPHeaderOssSecurityToken               = "X-Oss-Security-Token"
	HTTPHeaderOssServerSideEncryption        = "X-Oss-Server-Side-Encryption"
	HTTPHeaderOssServerSideEncryptionKeyID   = "X-Oss-Server-Side-Encryption-Key-Id"
	HTTPHeaderOssCopySource                  = "X-Oss-Copy-Source"
	HTTPHeaderOssCopySourceRange             = "X-Oss-Copy-Source-Range"
	HTTPHeaderOssCopySourceIfMatch           = "X-Oss-Copy-Source-If-Match"
	HTTPHeaderOssCopySourceIfNoneMatch       = "X-Oss-Copy-Source-If-None-Match"
	HTTPHeaderOssCopySourceIfModifiedSince   = "X-Oss-Copy-Source-If-Modified-Since"
	HTTPHeaderOssCopySourceIfUnmodifiedSince = "X-Oss-Copy-Source-If-Unmodified-Since"
	HTTPHeaderOssMetadataDirective           = "X-Oss-Metadata-Directive"
	HTTPHeaderOssNextAppendPosition          = "X-Oss-Next-Append-Position"
	HTTPHeaderOssRequestID                   = "X-Oss-Request-Id"
	HTTPHeaderOssCRC64                       = "X-Oss-Hash-Crc64ecma"
	HTTPHeaderOssSymlinkTarget               = "X-Oss-Symlink-Target"
	HTTPHeaderOssStorageClass                = "X-Oss-Storage-Class"
	HTTPHeaderOssCallback                    = "X-Oss-Callback"
	HTTPHeaderOssCallbackVar                 = "X-Oss-Callback-Var"
	HTTPHeaderOssRequester                   = "X-Oss-Request-Payer"
	HTTPHeaderOssTagging                     = "X-Oss-Tagging"
	HTTPHeaderOssTaggingDirective            = "X-Oss-Tagging-Directive"
	HTTPHeaderOssTrafficLimit                = "X-Oss-Traffic-Limit"
)

// HTTP Param
const (
	HTTPParamExpires       = "Expires"
	HTTPParamAccessKeyID   = "OSSAccessKeyId"
	HTTPParamSignature     = "Signature"
	HTTPParamSecurityToken = "security-token"
	HTTPParamPlaylistName  = "playlistName"
)

// Other constants
const (
	MaxPartSize = 5 * 1024 * 1024 * 1024 // Max part size, 5GB
	MinPartSize = 100 * 1024             // Min part size, 100KB

	FilePermMode = os.FileMode(0664) // Default file permission

	TempFilePrefix = "oss-go-temp-" // Temp file prefix
	TempFileSuffix = ".temp"        // Temp file suffix

	CheckpointFileSuffix = ".cp" // Checkpoint file suffix

	NullVersion = "null"

	Version = "v2.0.4" // Go SDK version
)

// FrameType
const (
	DataFrameType        = 8388609
	ContinuousFrameType  = 8388612
	EndFrameType         = 8388613
	MetaEndFrameCSVType  = 8388614
	MetaEndFrameJSONType = 8388615
)
