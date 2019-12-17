package oss

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"net/http"
	"time"
)

// The adapter class for Select object's response.
// The response consists of frames. Each frame has the following format:

// Type  |   Payload Length |  Header Checksum | Payload | Payload Checksum

// |<4-->|  <--4 bytes------><---4 bytes-------><-n/a-----><--4 bytes--------->
// And we have three kind of frames.
// Data Frame:
// Type:8388609
// Payload:   Offset    |    Data
//            <-8 bytes>

// Continuous Frame
// Type:8388612
// Payload: Offset  (8-bytes)

// End Frame
// Type:8388613
// Payload: Offset | total scanned bytes | http status code | error message
//     <-- 8bytes--><-----8 bytes--------><---4 bytes-------><---variabe--->

// SelectObjectResponse defines HTTP response from OSS SelectObject
type SelectObjectResponse struct {
	StatusCode          int
	Headers             http.Header
	Body                io.ReadCloser
	Frame               SelectObjectResult
	ReadTimeOut         uint
	ClientCRC32         uint32
	ServerCRC32         uint32
	WriterForCheckCrc32 hash.Hash32
	Finish              bool
}

func (sr *SelectObjectResponse) Read(p []byte) (n int, err error) {
	n, err = sr.readFrames(p)
	return
}

// Close http reponse body
func (sr *SelectObjectResponse) Close() error {
	return sr.Body.Close()
}

// PostSelectResult is the request of SelectObject
type PostSelectResult struct {
	Response *SelectObjectResponse
}

// readFrames is read Frame
func (sr *SelectObjectResponse) readFrames(p []byte) (int, error) {
	var nn int
	var err error
	var checkValid bool
	if sr.Frame.OutputRawData == true {
		nn, err = sr.Body.Read(p)
		return nn, err
	}

	if sr.Finish {
		return 0, io.EOF
	}

	for {
		// if this Frame is Readed, then not reading Header
		if sr.Frame.OpenLine != true {
			err = sr.analysisHeader()
			if err != nil {
				return nn, err
			}
		}

		if sr.Frame.FrameType == DataFrameType {
			n, err := sr.analysisData(p[nn:])
			if err != nil {
				return nn, err
			}
			nn += n

			// if this Frame is readed all data, then empty the Frame to read it with next frame
			if sr.Frame.ConsumedBytesLength == sr.Frame.PayloadLength-8 {
				checkValid, err = sr.checkPayloadSum()
				if err != nil || !checkValid {
					return nn, fmt.Errorf("%s", err.Error())
				}
				sr.emptyFrame()
			}

			if nn == len(p) {
				return nn, nil
			}
		} else if sr.Frame.FrameType == ContinuousFrameType {
			checkValid, err = sr.checkPayloadSum()
			if err != nil || !checkValid {
				return nn, fmt.Errorf("%s", err.Error())
			}
		} else if sr.Frame.FrameType == EndFrameType {
			err = sr.analysisEndFrame()
			if err != nil {
				return nn, err
			}
			checkValid, err = sr.checkPayloadSum()
			if checkValid {
				sr.Finish = true
			}
			return nn, err
		} else if sr.Frame.FrameType == MetaEndFrameCSVType {
			err = sr.analysisMetaEndFrameCSV()
			if err != nil {
				return nn, err
			}
			checkValid, err = sr.checkPayloadSum()
			if checkValid {
				sr.Finish = true
			}
			return nn, err
		} else if sr.Frame.FrameType == MetaEndFrameJSONType {
			err = sr.analysisMetaEndFrameJSON()
			if err != nil {
				return nn, err
			}
			checkValid, err = sr.checkPayloadSum()
			if checkValid {
				sr.Finish = true
			}
			return nn, err
		}
	}
	return nn, nil
}

type chanReadIO struct {
	readLen int
	err     error
}

func (sr *SelectObjectResponse) readLen(p []byte, timeOut time.Duration) (int, error) {
	r := sr.Body
	ch := make(chan chanReadIO, 1)
	defer close(ch)
	go func(p []byte) {
		var needReadLength int
		readChan := chanReadIO{}
		needReadLength = len(p)
		for {
			n, err := r.Read(p[readChan.readLen:needReadLength])
			readChan.readLen += n
			if err != nil {
				readChan.err = err
				ch <- readChan
				return
			}

			if readChan.readLen == needReadLength {
				break
			}
		}
		ch <- readChan
	}(p)

	select {
	case <-time.After(time.Second * timeOut):
		return 0, fmt.Errorf("requestId: %s, readLen timeout, timeout is %d(second),need read:%d", sr.Headers.Get(HTTPHeaderOssRequestID), timeOut, len(p))
	case result := <-ch:
		return result.readLen, result.err
	}
}

// analysisHeader is reading selectObject response body's header
func (sr *SelectObjectResponse) analysisHeader() error {
	headFrameByte := make([]byte, 20)
	_, err := sr.readLen(headFrameByte, time.Duration(sr.ReadTimeOut))
	if err != nil {
		return fmt.Errorf("requestId: %s, Read response frame header failure,err:%s", sr.Headers.Get(HTTPHeaderOssRequestID), err.Error())
	}

	frameTypeByte := headFrameByte[0:4]
	sr.Frame.Version = frameTypeByte[0]
	frameTypeByte[0] = 0
	bytesToInt(frameTypeByte, &sr.Frame.FrameType)

	if sr.Frame.FrameType != DataFrameType && sr.Frame.FrameType != ContinuousFrameType &&
		sr.Frame.FrameType != EndFrameType && sr.Frame.FrameType != MetaEndFrameCSVType && sr.Frame.FrameType != MetaEndFrameJSONType {
		return fmt.Errorf("requestId: %s, Unexpected frame type: %d", sr.Headers.Get(HTTPHeaderOssRequestID), sr.Frame.FrameType)
	}

	payloadLengthByte := headFrameByte[4:8]
	bytesToInt(payloadLengthByte, &sr.Frame.PayloadLength)
	headCheckSumByte := headFrameByte[8:12]
	bytesToInt(headCheckSumByte, &sr.Frame.HeaderCheckSum)
	byteOffset := headFrameByte[12:20]
	bytesToInt(byteOffset, &sr.Frame.Offset)
	sr.Frame.OpenLine = true

	err = sr.writerCheckCrc32(byteOffset)
	return err
}

// analysisData is reading the DataFrameType data of selectObject response body
func (sr *SelectObjectResponse) analysisData(p []byte) (int, error) {
	var needReadLength int32
	lenP := int32(len(p))
	restByteLength := sr.Frame.PayloadLength - 8 - sr.Frame.ConsumedBytesLength
	if lenP <= restByteLength {
		needReadLength = lenP
	} else {
		needReadLength = restByteLength
	}
	n, err := sr.readLen(p[:needReadLength], time.Duration(sr.ReadTimeOut))
	if err != nil {
		return n, fmt.Errorf("read frame data error,%s", err.Error())
	}
	sr.Frame.ConsumedBytesLength += int32(n)
	err = sr.writerCheckCrc32(p[:n])
	return n, err
}

// analysisEndFrame is reading the EndFrameType data of selectObject response body
func (sr *SelectObjectResponse) analysisEndFrame() error {
	var eF EndFrame
	payLoadBytes := make([]byte, sr.Frame.PayloadLength-8)
	_, err := sr.readLen(payLoadBytes, time.Duration(sr.ReadTimeOut))
	if err != nil {
		return fmt.Errorf("read end frame error:%s", err.Error())
	}
	bytesToInt(payLoadBytes[0:8], &eF.TotalScanned)
	bytesToInt(payLoadBytes[8:12], &eF.HTTPStatusCode)
	errMsgLength := sr.Frame.PayloadLength - 20
	eF.ErrorMsg = string(payLoadBytes[12 : errMsgLength+12])
	sr.Frame.EndFrame.TotalScanned = eF.TotalScanned
	sr.Frame.EndFrame.HTTPStatusCode = eF.HTTPStatusCode
	sr.Frame.EndFrame.ErrorMsg = eF.ErrorMsg
	err = sr.writerCheckCrc32(payLoadBytes)
	return err
}

// analysisMetaEndFrameCSV is reading the MetaEndFrameCSVType data of selectObject response body
func (sr *SelectObjectResponse) analysisMetaEndFrameCSV() error {
	var mCF MetaEndFrameCSV
	payLoadBytes := make([]byte, sr.Frame.PayloadLength-8)
	_, err := sr.readLen(payLoadBytes, time.Duration(sr.ReadTimeOut))
	if err != nil {
		return fmt.Errorf("read meta end csv frame error:%s", err.Error())
	}

	bytesToInt(payLoadBytes[0:8], &mCF.TotalScanned)
	bytesToInt(payLoadBytes[8:12], &mCF.Status)
	bytesToInt(payLoadBytes[12:16], &mCF.SplitsCount)
	bytesToInt(payLoadBytes[16:24], &mCF.RowsCount)
	bytesToInt(payLoadBytes[24:28], &mCF.ColumnsCount)
	errMsgLength := sr.Frame.PayloadLength - 36
	mCF.ErrorMsg = string(payLoadBytes[28 : errMsgLength+28])
	sr.Frame.MetaEndFrameCSV.ErrorMsg = mCF.ErrorMsg
	sr.Frame.MetaEndFrameCSV.TotalScanned = mCF.TotalScanned
	sr.Frame.MetaEndFrameCSV.Status = mCF.Status
	sr.Frame.MetaEndFrameCSV.SplitsCount = mCF.SplitsCount
	sr.Frame.MetaEndFrameCSV.RowsCount = mCF.RowsCount
	sr.Frame.MetaEndFrameCSV.ColumnsCount = mCF.ColumnsCount
	err = sr.writerCheckCrc32(payLoadBytes)
	return err
}

// analysisMetaEndFrameJSON is reading the MetaEndFrameJSONType data of selectObject response body
func (sr *SelectObjectResponse) analysisMetaEndFrameJSON() error {
	var mJF MetaEndFrameJSON
	payLoadBytes := make([]byte, sr.Frame.PayloadLength-8)
	_, err := sr.readLen(payLoadBytes, time.Duration(sr.ReadTimeOut))
	if err != nil {
		return fmt.Errorf("read meta end json frame error:%s", err.Error())
	}

	bytesToInt(payLoadBytes[0:8], &mJF.TotalScanned)
	bytesToInt(payLoadBytes[8:12], &mJF.Status)
	bytesToInt(payLoadBytes[12:16], &mJF.SplitsCount)
	bytesToInt(payLoadBytes[16:24], &mJF.RowsCount)
	errMsgLength := sr.Frame.PayloadLength - 32
	mJF.ErrorMsg = string(payLoadBytes[24 : errMsgLength+24])
	sr.Frame.MetaEndFrameJSON.ErrorMsg = mJF.ErrorMsg
	sr.Frame.MetaEndFrameJSON.TotalScanned = mJF.TotalScanned
	sr.Frame.MetaEndFrameJSON.Status = mJF.Status
	sr.Frame.MetaEndFrameJSON.SplitsCount = mJF.SplitsCount
	sr.Frame.MetaEndFrameJSON.RowsCount = mJF.RowsCount

	err = sr.writerCheckCrc32(payLoadBytes)
	return err
}

func (sr *SelectObjectResponse) checkPayloadSum() (bool, error) {
	payLoadChecksumByte := make([]byte, 4)
	n, err := sr.readLen(payLoadChecksumByte, time.Duration(sr.ReadTimeOut))
	if n == 4 {
		bytesToInt(payLoadChecksumByte, &sr.Frame.PayloadChecksum)
		sr.ServerCRC32 = sr.Frame.PayloadChecksum
		sr.ClientCRC32 = sr.WriterForCheckCrc32.Sum32()
		if sr.Frame.EnablePayloadCrc == true && sr.ServerCRC32 != 0 && sr.ServerCRC32 != sr.ClientCRC32 {
			return false, fmt.Errorf("RequestId: %s, Unexpected frame type: %d, client %d but server %d",
				sr.Headers.Get(HTTPHeaderOssRequestID), sr.Frame.FrameType, sr.ClientCRC32, sr.ServerCRC32)
		}
		return true, err
	}
	return false, fmt.Errorf("RequestId:%s, read checksum error:%s", sr.Headers.Get(HTTPHeaderOssRequestID), err.Error())
}

func (sr *SelectObjectResponse) writerCheckCrc32(p []byte) (err error) {
	err = nil
	if sr.Frame.EnablePayloadCrc == true {
		_, err = sr.WriterForCheckCrc32.Write(p)
	}
	return err
}

// emptyFrame is emptying SelectObjectResponse Frame information
func (sr *SelectObjectResponse) emptyFrame() {
	crcCalc := crc32.NewIEEE()
	sr.WriterForCheckCrc32 = crcCalc
	sr.Finish = false

	sr.Frame.ConsumedBytesLength = 0
	sr.Frame.OpenLine = false
	sr.Frame.Version = byte(0)
	sr.Frame.FrameType = 0
	sr.Frame.PayloadLength = 0
	sr.Frame.HeaderCheckSum = 0
	sr.Frame.Offset = 0
	sr.Frame.Data = ""

	sr.Frame.EndFrame.TotalScanned = 0
	sr.Frame.EndFrame.HTTPStatusCode = 0
	sr.Frame.EndFrame.ErrorMsg = ""

	sr.Frame.MetaEndFrameCSV.TotalScanned = 0
	sr.Frame.MetaEndFrameCSV.Status = 0
	sr.Frame.MetaEndFrameCSV.SplitsCount = 0
	sr.Frame.MetaEndFrameCSV.RowsCount = 0
	sr.Frame.MetaEndFrameCSV.ColumnsCount = 0
	sr.Frame.MetaEndFrameCSV.ErrorMsg = ""

	sr.Frame.MetaEndFrameJSON.TotalScanned = 0
	sr.Frame.MetaEndFrameJSON.Status = 0
	sr.Frame.MetaEndFrameJSON.SplitsCount = 0
	sr.Frame.MetaEndFrameJSON.RowsCount = 0
	sr.Frame.MetaEndFrameJSON.ErrorMsg = ""

	sr.Frame.PayloadChecksum = 0
}

// bytesToInt byte's array trans to int
func bytesToInt(b []byte, ret interface{}) {
	binBuf := bytes.NewBuffer(b)
	binary.Read(binBuf, binary.BigEndian, ret)
}
