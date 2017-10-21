# ChangeLog - Aliyun OSS SDK for Go

## 版本号：1.7.0 日期：2017-09-25
### 变更内容
 - 增加：DownloadFile支持CRC校验
 - 增加：STS测试用例

## 版本号：1.6.0 日期：2017-09-01
### 变更内容
 - 修复：URL中特殊字符的编码问题
 - 变更：不再支持Golang 1.4
 
## 版本号：1.5.1 日期：2017-08-04
### 变更内容
 - 修复：SignURL中Key编码的问题
 - 修复：DownloadFile下载完成后rename失败的问题
 
## 版本号：1.5.0 日期：2017-07-25
### 变更内容
 - 增加：支持生成URL签名
 - 增加：GetObject支持ResponseContentType等选项
 - 修复：DownloadFile去除分片小于5GB的限制
 - 修复：AppendObject在appendPosition不正确时发生panic

## 版本号：1.4.0 日期：2017-05-23
### 变更内容
 - 增加：支持符号链接symlink
 - 增加：支持RestoreObject
 - 增加：CreateBucket支持StorageClass
 - 增加：支持范围读NormalizedRange
 - 修复：IsObjectExist使用GetObjectMeta实现

## 版本号：1.3.0 日期：2017-01-13
### 变更内容
 - 增加：上传下载支持进度条功能

## 版本号：1.2.3 日期：2016-12-28
### 变更内容
 - 修复：每次请求使用一个http.Client修改为共用http.Client

## 版本号：1.2.2 日期：2016-12-10
### 变更内容
 - 修复：GetObjectToFile/DownloadFile使用临时文件下载，成功后重命名成下载文件
 - 修复：新建的下载文件权限修改为0664

## 版本号：1.2.1 日期：2016-11-11
### 变更内容
 - 修复：只有当OSS返回x-oss-hash-crc64ecma头部时，才对上传的文件进行CRC64完整性校验

## 版本号：1.2.0 日期：2016-10-18
### 变更内容
 - 增加：支持CRC64校验
 - 增加：支持指定Useragent
 - 修复：计算MD5占用内存大的问题
 - 修复：CopyObject时Object名称没有URL编码的问题

## 版本号：1.1.0 日期：2016-08-09
### 变更内容
 - 增加：支持代理服务器

## 版本号：1.0.0 日期：2016-06-24
### 变更内容
 - 增加：断点分片复制接口Bucket.CopyFile
 - 增加：Bucket间复制接口Bucket.CopyObjectTo、Bucket.CopyObjectFrom
 - 增加：Client.GetBucketInfo接口
 - 增加：Bucket.UploadPartCopy支持Bucket间复制
 - 修复：断点上传、断点下载出错后，协程不退出的Bug
 - 删除：接口Bucket.CopyObjectToBucket
