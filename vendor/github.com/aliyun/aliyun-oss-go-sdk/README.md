# Alibaba Cloud OSS SDK for Go

[![GitHub Version](https://badge.fury.io/gh/aliyun%2Faliyun-oss-go-sdk.svg)](https://badge.fury.io/gh/aliyun%2Faliyun-oss-go-sdk)
[![Build Status](https://travis-ci.org/aliyun/aliyun-oss-go-sdk.svg?branch=master)](https://travis-ci.org/aliyun/aliyun-oss-go-sdk)
[![Coverage Status](https://coveralls.io/repos/github/aliyun/aliyun-oss-go-sdk/badge.svg?branch=master)](https://coveralls.io/github/aliyun/aliyun-oss-go-sdk?branch=master)

## [README of Chinese](https://github.com/aliyun/aliyun-oss-go-sdk/blob/master/README-CN.md)

## About
> - This Go SDK is based on the official APIs of [Alibaba Cloud OSS](http://www.aliyun.com/product/oss/).
> - Alibaba Cloud Object Storage Service (OSS) is a cloud storage service provided by Alibaba Cloud, featuring massive capacity, security, a low cost, and high reliability. 
> - The OSS can store any type of files and therefore applies to various websites, development enterprises and developers.
> - With this SDK, you can upload, download and manage data on any app anytime and anywhere conveniently. 

## Version
> - Current version: 1.7.0. 

## Running Environment
> - Go 1.5 or above. 

## Installing
### Install the SDK through GitHub
> - Run the 'go get github.com/aliyun/aliyun-oss-go-sdk/oss' command to get the remote code package.
> - Use 'import "github.com/aliyun/aliyun-oss-go-sdk/oss"' in your code to introduce OSS Go SDK package.

## Getting Started
### List Bucket
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    lsRes, err := client.ListBuckets()
    if err != nil {
        // HandleError(err)
    }
    
    for _, bucket := range lsRes.Buckets {
        fmt.Println("Buckets:", bucket.Name)
    }
```

### Create Bucket
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    err = client.CreateBucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
```
    
### Delete Bucket
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    err = client.DeleteBucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
```

### Put Object
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    bucket, err := client.Bucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
    
    err = bucket.PutObjectFromFile("my-object", "LocalFile")
    if err != nil {
        // HandleError(err)
    }
```

### Get Object
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    bucket, err := client.Bucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
    
    err = bucket.GetObjectToFile("my-object", "LocalFile")
    if err != nil {
        // HandleError(err)
    }
```

### List Objects
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    bucket, err := client.Bucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
    
    lsRes, err := bucket.ListObjects()
    if err != nil {
        // HandleError(err)
    }
    
    for _, object := range lsRes.Objects {
        fmt.Println("Objects:", object.Key)
    }
```
    
### Delete Object
```go
    client, err := oss.New("Endpoint", "AccessKeyId", "AccessKeySecret")
    if err != nil {
        // HandleError(err)
    }
    
    bucket, err := client.Bucket("my-bucket")
    if err != nil {
        // HandleError(err)
    }
    
    err = bucket.DeleteObject("my-object")
    if err != nil {
        // HandleError(err)
    }
```

##  Complete Example
More example projects can be found at 'src\github.com\aliyun\aliyun-oss-go-sdk\sample' under the installation path of the OSS Go SDK (the first path of the GOPATH variable). The directory contains example projects. 
Or you can refer to the example objects in the sample directory under 'https://github.com/aliyun/aliyun-oss-go-sdk'.

### Running Example
> - Copy the example file. Go to the installation path of OSS Go SDK (the first path of the GOPATH variable), enter the code directory of the OSS Go SDK, namely 'src\github.com\aliyun\aliyun-oss-go-sdk',
and copy the sample directory and sample.go to the src directory of your test project.
> - Modify the  endpoint, AccessKeyId, AccessKeySecret and BucketName configuration settings in sample/config.go.
> - Run 'go run src/sample.go' under your project directory.

## Contacting us
> - [Alibaba Cloud OSS official website](http://oss.aliyun.com).
> - [Alibaba Cloud OSS official forum](http://bbs.aliyun.com).
> - [Alibaba Cloud OSS official documentation center](http://www.aliyun.com/product/oss#Docs).
> - Alibaba Cloud official technical support: [Submit a ticket](https://workorder.console.aliyun.com/#/ticket/createIndex). 

## Author
> - Yubin Bai.
> - Hǎiliàng Wáng.

## License
> - Apache License 2.0.
