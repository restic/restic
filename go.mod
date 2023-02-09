module github.com/restic/restic

replace (
	golang.org/x/text => golang.org/x/text v0.3.7
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.4.0
)

require (
	bazil.org/fuse v0.0.0-20200407214033-5883e5a4b512
	cloud.google.com/go/storage v1.16.0
	github.com/Azure/azure-sdk-for-go v55.6.0+incompatible
	github.com/aws/aws-sdk-go v1.38.21
	github.com/cenkalti/backoff/v4 v4.1.1
	github.com/cespare/xxhash/v2 v2.1.1
	github.com/elithrar/simple-scrypt v1.3.0
	github.com/go-ole/go-ole v1.2.5
	github.com/google/go-cmp v0.5.6
	github.com/hashicorp/golang-lru v0.5.4
	github.com/juju/ratelimit v1.0.1
	github.com/klauspost/compress v1.15.11
	github.com/kurin/blazer v0.5.4-0.20211030221322-ba894c124ac6
	github.com/minio/minio-go/v7 v7.0.14
	github.com/minio/sha256-simd v1.0.0
	github.com/ncw/swift/v2 v2.0.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.6.0
	github.com/pkg/sftp v1.13.2
	github.com/pkg/xattr v0.4.5
	github.com/restic/chunker v0.4.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
	golang.org/x/text v0.3.6
	google.golang.org/api v0.50.0
)

require (
	cloud.google.com/go v0.84.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.19 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.13 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dnaeon/go-vcr v1.2.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.2+incompatible // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/jstemmer/go-junit-report v0.9.1 // indirect
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/klauspost/cpuid/v2 v2.0.4 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/minio/md5-simd v1.1.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/xid v1.2.1 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/tools v0.1.4 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210624195500-8bfb893ecb84 // indirect
	google.golang.org/grpc v1.38.0 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/ini.v1 v1.62.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

go 1.18
