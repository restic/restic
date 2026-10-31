package drime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
)

const (
	defaultEndpoint = "https://app.drime.cloud/api/v1"

	// Drime silently limits the page size to 100 entries.
	listPageSize = 100

	folderType = "folder"
	maxRetries = 5
)

var (
	errNotFound = errors.New("file not found")
	errTooShort = errors.New("file is too short")
	errRange    = errors.New("requested range not satisfiable")

	errCorrupted = errors.New("file was corrupted during upload")
)

// apiError is returned for unexpected responses from the Drime API.
type apiError struct {
	Op      string
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("drime: %s: unexpected status %d: %s", e.Op, e.Status, e.Message)
}

// item describes a file or a folder.
type item struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	FileSize int64       `json:"file_size"`
	Type     string      `json:"type"`
}

type listing struct {
	CurrentPage int    `json:"current_page"`
	LastPage    int    `json:"last_page"`
	Data        []item `json:"data"`
}

type client struct {
	hc        *http.Client
	endpoint  string
	token     string
	workspace string
}

func (c *client) newRequest(ctx context.Context, method, p string, query url.Values, body io.Reader) (*http.Request, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("workspaceId", c.workspace)

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+p+"?"+query.Encode(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func parseError(op string, resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var e struct {
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	msg := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &e) == nil {
		msg = e.Message
		if len(e.Errors) > 0 {
			msg += " " + string(e.Errors)
		}
	}
	return &apiError{Op: op, Status: resp.StatusCode, Message: strings.TrimSpace(msg)}
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

// do sends the request built by mk and waits when the API rate limit is hit.
// A returned response always has a status below 300.
func (c *client) do(ctx context.Context, op string, mk func() (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := mk()
		if err != nil {
			return nil, err
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, op)
		}
		if resp.StatusCode < 300 {
			return resp, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			delay := retryDelay(resp, attempt)
			_ = resp.Body.Close()
			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		err = parseError(op, resp)
		_ = resp.Body.Close()
		return nil, err
	}
}

func (c *client) doJSON(ctx context.Context, op, method, p string, query url.Values, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		payload, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}

	resp, err := c.do(ctx, op, func() (*http.Request, error) {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := c.newRequest(ctx, method, p, query, body)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, nil
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return errors.Wrap(json.NewDecoder(resp.Body).Decode(out), op)
}

// list calls fn for all entries in the folder with the given id, an empty id
// is the root of the drive. If name is not empty, the server filters by name.
func (c *client) list(ctx context.Context, folderID, name string, foldersOnly bool, fn func(item) error) error {
	for page := 1; ; page++ {
		q := url.Values{}
		if folderID != "" {
			q.Set("folderId", folderID)
		}
		if foldersOnly {
			q.Set("type", folderType)
		}
		if name != "" {
			q.Set("query", name)
		}
		q.Set("perPage", strconv.Itoa(listPageSize))
		q.Set("page", strconv.Itoa(page))

		var l listing
		err := c.doJSON(ctx, "list", http.MethodGet, "/drive/file-entries", q, nil, &l)
		if err != nil {
			return err
		}
		// Drime serves the last page again instead of failing when a
		// directory contains more entries than it is willing to list.
		if l.CurrentPage != page {
			return errors.New("drime: directory has more entries than can be listed")
		}

		for _, it := range l.Data {
			if err := fn(it); err != nil {
				return err
			}
		}
		if page >= l.LastPage || len(l.Data) == 0 {
			return nil
		}
	}
}

func parentID(id string) any {
	if id == "" {
		return nil
	}
	return json.Number(id)
}

func (c *client) mkdir(ctx context.Context, parent, name string) (string, error) {
	var res struct {
		Folder item `json:"folder"`
	}
	err := c.doJSON(ctx, "mkdir", http.MethodPost, "/folders", nil,
		map[string]any{"name": name, "parentId": parentID(parent)}, &res)
	if err != nil {
		return "", err
	}
	return res.Folder.ID.String(), nil
}

func (c *client) remove(ctx context.Context, id string) error {
	err := c.doJSON(ctx, "remove", http.MethodPost, "/file-entries/delete", nil,
		map[string]any{"entryIds": []json.Number{json.Number(id)}, "deleteForever": true}, nil)

	// unknown ids are reported as a validation error
	var ae *apiError
	if errors.As(err, &ae) && (ae.Status == http.StatusUnprocessableEntity || ae.Status == http.StatusNotFound) {
		return errNotFound
	}
	return err
}

type entryResponse struct {
	FileEntry item `json:"fileEntry"`
}

// upload stores a small file using a single request.
func (c *client) upload(ctx context.Context, folderID, name string, data []byte) (item, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("parentId", folderID)
	_ = mw.WriteField("relativePath", name)
	_ = mw.WriteField("workspaceId", c.workspace)

	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	hdr.Set("Content-Type", "application/octet-stream")
	w, err := mw.CreatePart(hdr)
	if err != nil {
		return item{}, err
	}
	if _, err = w.Write(data); err != nil {
		return item{}, err
	}
	if err = mw.Close(); err != nil {
		return item{}, err
	}

	resp, err := c.do(ctx, "upload", func() (*http.Request, error) {
		req, err := c.newRequest(ctx, http.MethodPost, "/uploads", nil, bytes.NewReader(buf.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return req, nil
	})
	if err != nil {
		return item{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var res entryResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return item{}, errors.Wrap(err, "upload")
	}
	return res.FileEntry, nil
}

type multipartUpload struct {
	UploadID string `json:"uploadId"`
	Key      string `json:"key"`
}

type completedPart struct {
	ETag       string `json:"ETag"`
	PartNumber int    `json:"PartNumber"`
}

func (c *client) multipartCreate(ctx context.Context, folderID, name string, size int64) (multipartUpload, error) {
	var res multipartUpload
	err := c.doJSON(ctx, "multipart create", http.MethodPost, "/s3/multipart/create", nil, map[string]any{
		"filename":     name,
		"mime":         "application/octet-stream",
		"size":         size,
		"extension":    extension(name),
		"parentId":     parentID(folderID),
		"relativePath": name,
		"workspaceId":  c.workspace,
	}, &res)
	return res, err
}

func extension(name string) string {
	if ext := strings.TrimPrefix(path.Ext(name), "."); ext != "" {
		return ext
	}
	return "bin"
}

// multipartPart uploads a single part and returns its ETag.
func (c *client) multipartPart(ctx context.Context, up multipartUpload, number int, data []byte) (string, error) {
	var signed struct {
		URLs []struct {
			URL string `json:"url"`
		} `json:"urls"`
	}
	err := c.doJSON(ctx, "multipart sign", http.MethodPost, "/s3/multipart/batch-sign-part-urls", nil,
		map[string]any{"uploadId": up.UploadID, "key": up.Key, "partNumbers": []int{number}}, &signed)
	if err != nil {
		return "", err
	}
	if len(signed.URLs) != 1 {
		return "", errors.New("drime: multipart sign: no url returned")
	}

	// the presigned url must not receive the Drime credentials
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.URLs[0].URL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "multipart upload")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return "", parseError("multipart upload", resp)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("drime: multipart upload: no ETag returned")
	}
	return etag, nil
}

func (c *client) multipartComplete(ctx context.Context, folderID, name string, up multipartUpload, size int64, parts []completedPart) (item, error) {
	err := c.doJSON(ctx, "multipart complete", http.MethodPost, "/s3/multipart/complete", nil,
		map[string]any{"uploadId": up.UploadID, "key": up.Key, "parts": parts}, nil)
	if err != nil {
		return item{}, err
	}

	var res entryResponse
	err = c.doJSON(ctx, "multipart entry", http.MethodPost, "/s3/entries", nil, map[string]any{
		"clientMime":      "application/octet-stream",
		"clientName":      name,
		"filename":        path.Base(up.Key),
		"size":            size,
		"clientExtension": extension(name),
		"parentId":        parentID(folderID),
		"relativePath":    name,
		"workspaceId":     c.workspace,
	}, &res)
	return res.FileEntry, err
}

// verify asks Drime to hash the stored file and compare the result with the
// hex encoded SHA-256 sum.
func (c *client) verify(ctx context.Context, id, sum string) (verified bool, serverHash string, err error) {
	var res struct {
		Verified   bool   `json:"verified"`
		ServerHash string `json:"serverHash"`
	}
	err = c.doJSON(ctx, "verify", http.MethodPost, "/file-entries/"+url.PathEscape(id)+"/verify-integrity", nil,
		map[string]string{"sha256": sum}, &res)
	return res.Verified, res.ServerHash, err
}

func (c *client) multipartAbort(ctx context.Context, up multipartUpload) {
	_ = c.doJSON(ctx, "multipart abort", http.MethodPost, "/s3/multipart/abort", nil,
		map[string]any{"uploadId": up.UploadID, "key": up.Key}, nil)
}

// download returns a reader for the file with the given id. The API redirects
// to a presigned storage url which supports range requests.
func (c *client) download(ctx context.Context, id string, offset int64, length int) (io.ReadCloser, error) {
	ranged := offset > 0 || length > 0

	resp, err := c.do(ctx, "download", func() (*http.Request, error) {
		req, err := c.newRequest(ctx, http.MethodGet, "/file-entries/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Del("Accept")
		switch {
		case length > 0:
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length)-1))
		case offset > 0:
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		return req, nil
	})
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return nil, errNotFound
			case http.StatusRequestedRangeNotSatisfiable:
				return nil, errors.Wrap(errRange, ae.Message)
			}
		}
		return nil, err
	}

	if ranged && resp.StatusCode == http.StatusOK {
		// the range was ignored, skip to the requested offset
		if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
			_ = resp.Body.Close()
			return nil, errors.Wrap(errRange, "range ignored by server")
		}
	}
	return resp.Body, nil
}
