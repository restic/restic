package oss

import (
	"crypto/md5"
	"encoding/base64"
	"hash/crc64"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	. "gopkg.in/check.v1"
)

type OssCrcSuite struct {
	client *Client
	bucket *Bucket
}

var _ = Suite(&OssCrcSuite{})

// Run once when the suite starts running
func (s *OssCrcSuite) SetUpSuite(c *C) {
	client, err := New(endpoint, accessID, accessKey)
	c.Assert(err, IsNil)
	s.client = client

	s.client.CreateBucket(bucketName)
	time.Sleep(5 * time.Second)

	bucket, err := s.client.Bucket(bucketName)
	c.Assert(err, IsNil)
	s.bucket = bucket

	testLogger.Println("test crc started")
}

// Run before each test or benchmark starts running
func (s *OssCrcSuite) TearDownSuite(c *C) {
	// Delete Part
	lmur, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)

	for _, upload := range lmur.Uploads {
		var imur = InitiateMultipartUploadResult{Bucket: s.bucket.BucketName,
			Key: upload.Key, UploadID: upload.UploadID}
		err = s.bucket.AbortMultipartUpload(imur)
		c.Assert(err, IsNil)
	}

	// Delete Objects
	lor, err := s.bucket.ListObjects()
	c.Assert(err, IsNil)

	for _, object := range lor.Objects {
		err = s.bucket.DeleteObject(object.Key)
		c.Assert(err, IsNil)
	}

	testLogger.Println("test crc completed")
}

// Run after each test or benchmark runs
func (s *OssCrcSuite) SetUpTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// Run once after all tests or benchmarks have finished running
func (s *OssCrcSuite) TearDownTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// TestCRCGolden 测试OSS实现的CRC64
func (s *OssCrcSuite) TestCRCGolden(c *C) {
	type crcTest struct {
		out uint64
		in  string
	}

	var crcGolden = []crcTest{
		{0x0, ""},
		{0x3420000000000000, "a"},
		{0x36c4200000000000, "ab"},
		{0x3776c42000000000, "abc"},
		{0x336776c420000000, "abcd"},
		{0x32d36776c4200000, "abcde"},
		{0x3002d36776c42000, "abcdef"},
		{0x31b002d36776c420, "abcdefg"},
		{0xe21b002d36776c4, "abcdefgh"},
		{0x8b6e21b002d36776, "abcdefghi"},
		{0x7f5b6e21b002d367, "abcdefghij"},
		{0x8ec0e7c835bf9cdf, "Discard medicine more than two years old."},
		{0xc7db1759e2be5ab4, "He who has a shady past knows that nice guys finish last."},
		{0xfbf9d9603a6fa020, "I wouldn't marry him with a ten foot pole."},
		{0xeafc4211a6daa0ef, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
		{0x3e05b21c7a4dc4da, "The days of the digital watch are numbered.  -Tom Stoppard"},
		{0x5255866ad6ef28a6, "Nepal premier won't resign."},
		{0x8a79895be1e9c361, "For every action there is an equal and opposite government program."},
		{0x8878963a649d4916, "His money is twice tainted: 'taint yours and 'taint mine."},
		{0xa7b9d53ea87eb82f, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
		{0xdb6805c0966a2f9c, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
		{0xf3553c65dacdadd2, "size:  a.out:  bad magic"},
		{0x9d5e034087a676b9, "The major problem is with sendmail.  -Mark Horton"},
		{0xa6db2d7f8da96417, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
		{0x325e00cd2fe819f9, "If the enemy is within range, then so are you."},
		{0x88c6600ce58ae4c6, "It's well we cannot hear the screams/That we create in others' dreams."},
		{0x28c4a3f3b769e078, "You remind me of a TV show, but that's all right: I watch it anyway."},
		{0xa698a34c9d9f1dca, "C is as portable as Stonehedge!!"},
		{0xf6c1e2a8c26c5cfc, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
		{0xd402559dfe9b70c, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
		{0xdb6efff26aa94946, "How can you write a big system without C++?  -Paul Glick"},
	}

	var tab = crc64.MakeTable(crc64.ISO)

	for i := 0; i < len(crcGolden); i++ {
		golden := crcGolden[i]
		crc := NewCRC(tab, 0)
		io.WriteString(crc, golden.in)
		sum := crc.Sum64()

		c.Assert(sum, Equals, golden.out)
	}
}

// testCRC64Combine test crc64 on vector[0..pos] which should have CRC-64 crc.
// Also test CRC64Combine on vector[] split in two.
func testCRC64Combine(c *C, str string, pos int, crc uint64) {
	tabECMA := crc64.MakeTable(crc64.ECMA)

	// test crc64
	hash := crc64.New(tabECMA)
	io.WriteString(hash, str)
	crc1 := hash.Sum64()
	c.Assert(crc1, Equals, crc)

	// test crc64 combine
	hash = crc64.New(tabECMA)
	io.WriteString(hash, str[0:pos])
	crc1 = hash.Sum64()

	hash = crc64.New(tabECMA)
	io.WriteString(hash, str[pos:len(str)])
	crc2 := hash.Sum64()

	crc1 = CRC64Combine(crc1, crc2, uint64(len(str)-pos))
	c.Assert(crc1, Equals, crc)
}

// TestCRCGolden 测试CRC64Combine
func (s *OssCrcSuite) TestCRCCombine(c *C) {
	str := "123456789"
	testCRC64Combine(c, str, (len(str)+1)>>1, 0x995DC9BBDF1939FA)

	str = "This is a test of the emergency broadcast system."
	testCRC64Combine(c, str, (len(str)+1)>>1, 0x27DB187FC15BBC72)
}

// TestCRCGolden 测试CRC64Combine
func (s *OssCrcSuite) TestCRCRepeatedCombine(c *C) {
	tab := crc64.MakeTable(crc64.ECMA)
	str := "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"

	for i := 0; i <= len(str); i++ {
		hash := crc64.New(tab)
		io.WriteString(hash, string(str[0:i]))
		prev := hash.Sum64()

		hash = crc64.New(tab)
		io.WriteString(hash, string(str[i:len(str)]))
		post := hash.Sum64()

		crc := CRC64Combine(prev, post, uint64(len(str)-i))
		testLogger.Println("TestCRCRepeatedCombine:", prev, post, crc, i, len(str))
		c.Assert(crc == 0x7AD25FAFA1710407, Equals, true)
	}
}

// TestCRCGolden 测试CRC64Combine
func (s *OssCrcSuite) TestCRCRandomCombine(c *C) {
	tab := crc64.MakeTable(crc64.ECMA)
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"

	body, err := ioutil.ReadFile(fileName)
	c.Assert(err, IsNil)

	for i := 0; i < 10; i++ {
		fileParts, err := SplitFileByPartNum(fileName, 1+rand.Intn(9999))
		c.Assert(err, IsNil)

		var crc uint64
		for _, part := range fileParts {
			calc := NewCRC(tab, 0)
			calc.Write(body[part.Offset : part.Offset+part.Size])
			crc = CRC64Combine(crc, calc.Sum64(), (uint64)(part.Size))
		}

		testLogger.Println("TestCRCRandomCombine:", crc, i, fileParts)
		c.Assert(crc == 0x2B612D24FFF64222, Equals, true)
	}
}

// TestEnableCRCAndMD5 开启MD5和CRC校验
func (s *OssCrcSuite) TestEnableCRCAndMD5(c *C) {
	objectName := objectNamePrefix + "tecam"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	newFileName := "BingWallpaper-2015-11-07-2.jpg"
	objectValue := "空山新雨后，天气晚来秋。明月松间照，清泉石上流。竹喧归浣女，莲动下渔舟。随意春芳歇，王孙自可留。"

	client, err := New(endpoint, accessID, accessKey, EnableCRC(true), EnableMD5(true), MD5ThresholdCalcInMemory(200*1024))
	c.Assert(err, IsNil)
	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	// PutObject
	err = bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// GetObject
	body, err := bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(body)
	c.Assert(err, IsNil)
	body.Close()

	// GetObjectWithCRC
	getResult, err := bucket.DoGetObject(&GetObjectRequest{objectName}, nil)
	c.Assert(err, IsNil)
	str, err := readBody(getResult.Response.Body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)
	c.Assert(getResult.ClientCRC.Sum64(), Equals, getResult.ServerCRC)

	// PutObjectFromFile
	err = bucket.PutObjectFromFile(objectName, fileName)
	c.Assert(err, IsNil)

	// GetObjectToFile
	err = bucket.GetObjectToFile(objectName, newFileName)
	c.Assert(err, IsNil)
	eq, err := compareFiles(fileName, newFileName)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// DeleteObject
	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// AppendObject
	var nextPos int64
	nextPos, err = bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)
	nextPos, err = bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)

	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	request := &AppendObjectRequest{
		ObjectKey: objectName,
		Reader:    strings.NewReader(objectValue),
		Position:  0,
	}
	appendResult, err := bucket.DoAppendObject(request, []Option{InitCRC(0)})
	c.Assert(err, IsNil)
	request.Position = appendResult.NextPosition
	appendResult, err = bucket.DoAppendObject(request, []Option{InitCRC(appendResult.CRC)})
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	//	MultipartUpload
	chunks, err := SplitFileByPartSize(fileName, 100*1024)
	imurUpload, err := bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)
	var partsUpload []UploadPart

	for _, chunk := range chunks {
		part, err := bucket.UploadPartFromFile(imurUpload, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsUpload = append(partsUpload, part)
	}

	_, err = bucket.CompleteMultipartUpload(imurUpload, partsUpload)
	c.Assert(err, IsNil)

	// Check MultipartUpload
	err = bucket.GetObjectToFile(objectName, newFileName)
	c.Assert(err, IsNil)
	eq, err = compareFiles(fileName, newFileName)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// DeleteObjects
	_, err = bucket.DeleteObjects([]string{objectName})
	c.Assert(err, IsNil)
}

// TestDisableCRCAndMD5 关闭MD5和CRC校验
func (s *OssCrcSuite) TestDisableCRCAndMD5(c *C) {
	objectName := objectNamePrefix + "tdcam"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	newFileName := "BingWallpaper-2015-11-07-3.jpg"
	objectValue := "中岁颇好道，晚家南山陲。兴来每独往，胜事空自知。行到水穷处，坐看云起时。偶然值林叟，谈笑无还期。"

	client, err := New(endpoint, accessID, accessKey, EnableCRC(false), EnableMD5(false))
	c.Assert(err, IsNil)
	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	// PutObject
	err = bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// GetObject
	body, err := bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(body)
	c.Assert(err, IsNil)
	body.Close()

	// GetObjectWithCRC
	getResult, err := bucket.DoGetObject(&GetObjectRequest{objectName}, nil)
	c.Assert(err, IsNil)
	str, err := readBody(getResult.Response.Body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// PutObjectFromFile
	err = bucket.PutObjectFromFile(objectName, fileName)
	c.Assert(err, IsNil)

	// GetObjectToFile
	err = bucket.GetObjectToFile(objectName, newFileName)
	c.Assert(err, IsNil)
	eq, err := compareFiles(fileName, newFileName)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// DeleteObject
	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// AppendObject
	var nextPos int64
	nextPos, err = bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)
	nextPos, err = bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)

	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	request := &AppendObjectRequest{
		ObjectKey: objectName,
		Reader:    strings.NewReader(objectValue),
		Position:  0,
	}
	appendResult, err := bucket.DoAppendObject(request, []Option{InitCRC(0)})
	c.Assert(err, IsNil)
	request.Position = appendResult.NextPosition
	appendResult, err = bucket.DoAppendObject(request, []Option{InitCRC(appendResult.CRC)})
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	//	MultipartUpload
	chunks, err := SplitFileByPartSize(fileName, 100*1024)
	imurUpload, err := bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)
	var partsUpload []UploadPart

	for _, chunk := range chunks {
		part, err := bucket.UploadPartFromFile(imurUpload, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsUpload = append(partsUpload, part)
	}

	_, err = bucket.CompleteMultipartUpload(imurUpload, partsUpload)
	c.Assert(err, IsNil)

	// Check MultipartUpload
	err = bucket.GetObjectToFile(objectName, newFileName)
	c.Assert(err, IsNil)
	eq, err = compareFiles(fileName, newFileName)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// DeleteObjects
	_, err = bucket.DeleteObjects([]string{objectName})
	c.Assert(err, IsNil)
}

// TestSpecifyContentMD5 指定MD5
func (s *OssCrcSuite) TestSpecifyContentMD5(c *C) {
	objectName := objectNamePrefix + "tdcam"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	objectValue := "积雨空林烟火迟，蒸藜炊黍饷东菑。漠漠水田飞白鹭，阴阴夏木啭黄鹂。山中习静观朝槿，松下清斋折露葵。野老与人争席罢，海鸥何事更相疑。"

	mh := md5.Sum([]byte(objectValue))
	md5B64 := base64.StdEncoding.EncodeToString(mh[:])

	// PutObject
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue), ContentMD5(md5B64))
	c.Assert(err, IsNil)

	// PutObjectFromFile
	file, err := os.Open(fileName)
	md5 := md5.New()
	io.Copy(md5, file)
	mdHex := base64.StdEncoding.EncodeToString(md5.Sum(nil)[:])
	err = s.bucket.PutObjectFromFile(objectName, fileName, ContentMD5(mdHex))
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// AppendObject
	var nextPos int64
	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)
	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	request := &AppendObjectRequest{
		ObjectKey: objectName,
		Reader:    strings.NewReader(objectValue),
		Position:  0,
	}
	appendResult, err := s.bucket.DoAppendObject(request, []Option{InitCRC(0)})
	c.Assert(err, IsNil)
	request.Position = appendResult.NextPosition
	appendResult, err = s.bucket.DoAppendObject(request, []Option{InitCRC(appendResult.CRC)})
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	//	MultipartUpload
	imurUpload, err := s.bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)

	var partsUpload []UploadPart
	part, err := s.bucket.UploadPart(imurUpload, strings.NewReader(objectValue), (int64)(len([]byte(objectValue))), 1)
	c.Assert(err, IsNil)
	partsUpload = append(partsUpload, part)

	_, err = s.bucket.CompleteMultipartUpload(imurUpload, partsUpload)
	c.Assert(err, IsNil)

	// DeleteObject
	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestCopyObjectToOrFromNegative
func (s *OssCrcSuite) TestAppendObjectNegative(c *C) {
	objectName := objectNamePrefix + "taoncrc"
	objectValue := "空山不见人，但闻人语响。返影入深林，复照青苔上。"

	nextPos, err := s.bucket.AppendObject(objectName, strings.NewReader(objectValue), 0, InitCRC(0))
	c.Assert(err, IsNil)

	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader(objectValue), nextPos, InitCRC(0))
	c.Assert(err, NotNil)
	c.Assert(strings.HasPrefix(err.Error(), "oss: the crc"), Equals, true)
}
