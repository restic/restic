package oss

import (
	"bytes"
	"encoding/xml"
	"hash/crc32"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

// CreateSelectCsvObjectMeta is Creating csv object meta
//
// key        		the object key.
// csvMeta	  		the csv file meta
// options    		the options for create csv Meta  of the object.
//
// MetaEndFrameCSV 	the csv file meta info
// error    		it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) CreateSelectCsvObjectMeta(key string, csvMeta CsvMetaRequest, options ...Option) (MetaEndFrameCSV, error) {
	var endFrame MetaEndFrameCSV
	params := map[string]interface{}{}
	params["x-oss-process"] = "csv/meta"

	csvMeta.encodeBase64()
	bs, err := xml.Marshal(csvMeta)
	if err != nil {
		return endFrame, err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	resp, err := bucket.DoPostSelectObject(key, params, buffer, options...)
	if err != nil {
		return endFrame, err
	}
	defer resp.Body.Close()

	_, err = ioutil.ReadAll(resp)

	return resp.Frame.MetaEndFrameCSV, err
}

// CreateSelectJsonObjectMeta is Creating json object meta
//
// key        			the object key.
// csvMeta	  			the json file meta
// options    			the options for create json Meta  of the object.
//
// MetaEndFrameJSON 	the json file meta info
// error    			it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) CreateSelectJsonObjectMeta(key string, jsonMeta JsonMetaRequest, options ...Option) (MetaEndFrameJSON, error) {
	var endFrame MetaEndFrameJSON
	params := map[string]interface{}{}
	params["x-oss-process"] = "json/meta"

	bs, err := xml.Marshal(jsonMeta)
	if err != nil {
		return endFrame, err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	resp, err := bucket.DoPostSelectObject(key, params, buffer, options...)
	if err != nil {
		return endFrame, err
	}
	defer resp.Body.Close()

	_, err = ioutil.ReadAll(resp)

	return resp.Frame.MetaEndFrameJSON, err
}

// SelectObject is the select object api, approve csv and json file.
//
// key        		the object key.
// selectReq	  	the request data for select object
// options    		the options for select file of the object.
//
// o.ReadCloser 	reader instance for reading data from response. It must be called close() after the usage and only valid when error is nil.
// error    		it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) SelectObject(key string, selectReq SelectRequest, options ...Option) (io.ReadCloser, error) {
	params := map[string]interface{}{}
	if selectReq.InputSerializationSelect.JsonBodyInput.JsonIsEmpty() {
		params["x-oss-process"] = "csv/select" // default select csv file
	} else {
		params["x-oss-process"] = "json/select"
	}
	selectReq.encodeBase64()
	bs, err := xml.Marshal(selectReq)
	if err != nil {
		return nil, err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)
	resp, err := bucket.DoPostSelectObject(key, params, buffer, options...)
	if err != nil {
		return nil, err
	}
	if selectReq.OutputSerializationSelect.EnablePayloadCrc != nil && *selectReq.OutputSerializationSelect.EnablePayloadCrc == true {
		resp.Frame.EnablePayloadCrc = true
	}
	resp.Frame.OutputRawData = strings.ToUpper(resp.Headers.Get("x-oss-select-output-raw")) == "TRUE"

	return resp, err
}

// DoPostSelectObject is the SelectObject/CreateMeta api, approve csv and json file.
//
// key        			the object key.
// params	  			the resource of oss approve csv/meta, json/meta, csv/select, json/select.
// buf					the request data trans to buffer.
// options    			the options for select file of the object.
//
// SelectObjectResponse 	the response of select object.
// error    			it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoPostSelectObject(key string, params map[string]interface{}, buf *bytes.Buffer, options ...Option) (*SelectObjectResponse, error) {
	resp, err := bucket.do("POST", key, params, options, buf, nil)
	if err != nil {
		return nil, err
	}

	result := &SelectObjectResponse{
		Body:       resp.Body,
		StatusCode: resp.StatusCode,
		Frame:      SelectObjectResult{},
	}
	result.Headers = resp.Headers
	// result.Frame = SelectObjectResult{}
	result.ReadTimeOut = bucket.getConfig().Timeout

	// Progress
	listener := getProgressListener(options)

	// CRC32
	crcCalc := crc32.NewIEEE()
	result.WriterForCheckCrc32 = crcCalc
	result.Body = TeeReader(resp.Body, nil, 0, listener, nil)

	err = checkRespCode(resp.StatusCode, []int{http.StatusPartialContent, http.StatusOK})

	return result, err
}

// SelectObjectIntoFile is the selectObject to file api
//
// key        	the object key.
// fileName	  	saving file's name to localstation.
// selectReq	  	the request data for select object
// options 		the options for select file of the object.
//
// error    	it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) SelectObjectIntoFile(key, fileName string, selectReq SelectRequest, options ...Option) error {
	tempFilePath := fileName + TempFileSuffix

	params := map[string]interface{}{}
	if selectReq.InputSerializationSelect.JsonBodyInput.JsonIsEmpty() {
		params["x-oss-process"] = "csv/select" // default select csv file
	} else {
		params["x-oss-process"] = "json/select"
	}
	selectReq.encodeBase64()
	bs, err := xml.Marshal(selectReq)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)
	resp, err := bucket.DoPostSelectObject(key, params, buffer, options...)
	if err != nil {
		return err
	}
	defer resp.Close()

	// If the local file does not exist, create a new one. If it exists, overwrite it.
	fd, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, FilePermMode)
	if err != nil {
		return err
	}

	// Copy the data to the local file path.
	_, err = io.Copy(fd, resp)
	fd.Close()
	if err != nil {
		return err
	}

	return os.Rename(tempFilePath, fileName)
}
