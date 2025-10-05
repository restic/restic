package azure

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	azContainer "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// Backend stores data on an azure endpoint.
type Backend struct {
	cfg          Config
	container    *azContainer.Client
	connections  uint
	prefix       string
	listMaxItems int
	layout.Layout

	accessTier blob.AccessTier
}

const saveLargeSize = 256 * 1024 * 1024
const defaultListMaxItems = 5000

// make sure that *Backend implements backend.Backend
var _ backend.Backend = &Backend{}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("azure", ParseConfig, location.NoPassword, Create, Open)
}

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)
	var client *azContainer.Client
	var err error

	var endpointSuffix string
	if cfg.EndpointSuffix != "" {
		endpointSuffix = cfg.EndpointSuffix
	} else {
		endpointSuffix = "core.windows.net"
	}

	if cfg.AccountName == "" {
		return nil, errors.Fatalf("unable to open Azure backend: Account name ($AZURE_ACCOUNT_NAME) is empty")
	}

	url := fmt.Sprintf("https://%s.blob.%s/%s", cfg.AccountName, endpointSuffix, cfg.Container)
	opts := &azContainer.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: &http.Client{Transport: rt},
		},
	}

	if cfg.AccountKey.String() != "" {
		// We have an account key value, find the BlobServiceClient
		// from with a BasicClient
		debug.Log(" - using account key")
		cred, err := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey.Unwrap())
		if err != nil {
			return nil, errors.Wrap(err, "NewSharedKeyCredential")
		}

		client, err = azContainer.NewClientWithSharedKeyCredential(url, cred, opts)

		if err != nil {
			return nil, errors.Wrap(err, "NewClientWithSharedKeyCredential")
		}
	} else if cfg.AccountSAS.String() != "" {
		// Get the client using the SAS Token as authentication, this
		// is longer winded than above because the SDK wants a URL for the Account
		// if your using a SAS token, and not just the account name
		// we (as per the SDK ) assume the default Azure portal.
		// https://github.com/Azure/azure-storage-blob-go/issues/130
		debug.Log(" - using sas token")
		sas := cfg.AccountSAS.Unwrap()

		// strip query sign prefix
		if sas[0] == '?' {
			sas = sas[1:]
		}

		urlWithSAS := fmt.Sprintf("%s?%s", url, sas)

		client, err = azContainer.NewClientWithNoCredential(urlWithSAS, opts)
		if err != nil {
			return nil, errors.Wrap(err, "NewAccountSASClientFromEndpointToken")
		}
	} else {
		var cred azcore.TokenCredential

		if cfg.ForceCliCredential {
			debug.Log(" - using AzureCLICredential")
			cred, err = azidentity.NewAzureCLICredential(nil)
			if err != nil {
				return nil, errors.Wrap(err, "NewAzureCLICredential")
			}
		} else {
			debug.Log(" - using DefaultAzureCredential")
			cred, err = azidentity.NewDefaultAzureCredential(nil)
			if err != nil {
				return nil, errors.Wrap(err, "NewDefaultAzureCredential")
			}
		}

		client, err = azContainer.NewClient(url, cred, opts)
		if err != nil {
			return nil, errors.Wrap(err, "NewClient")
		}
	}

	var accessTier blob.AccessTier
	// if the access tier is not supported, then we will not set the access tier; during the upload process,
	// the value will be inferred from the default configured on the storage account.
	for _, tier := range supportedAccessTiers() {
		if strings.EqualFold(string(tier), cfg.AccessTier) {
			accessTier = tier
			debug.Log(" - using access tier %v", accessTier)
			break
		}
	}

	be := &Backend{
		container:    client,
		cfg:          cfg,
		connections:  cfg.Connections,
		Layout:       layout.NewDefaultLayout(cfg.Prefix, path.Join),
		listMaxItems: defaultListMaxItems,
		accessTier:   accessTier,
	}

	return be, nil
}

func supportedAccessTiers() []blob.AccessTier {
	return []blob.AccessTier{blob.AccessTierHot, blob.AccessTierCool, blob.AccessTierCold, blob.AccessTierArchive}
}

// Open opens the Azure backend at specified container.
func Open(_ context.Context, cfg Config, rt http.RoundTripper, _ func(string, ...interface{})) (*Backend, error) {
	return open(cfg, rt)
}

// Create opens the Azure backend at specified container and creates the container if
// it does not exist yet.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper, _ func(string, ...interface{})) (*Backend, error) {
	be, err := open(cfg, rt)

	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	_, err = be.container.GetProperties(ctx, &azContainer.GetPropertiesOptions{})

	if err != nil && bloberror.HasCode(err, bloberror.ContainerNotFound) {
		_, err = be.container.Create(ctx, &azContainer.CreateOptions{})

		if err != nil {
			return nil, errors.Wrap(err, "container.Create")
		}
	} else if err != nil && bloberror.HasCode(err, bloberror.AuthorizationFailure) {
		// We ignore this Auth. Failure, as the failure is related to the type
		// of SAS/SAT, not an actual real failure. If the token is invalid, we
		// fail later on anyway.
		// For details see Issue #4004.
		debug.Log("Ignoring AuthorizationFailure when calling GetProperties")
	} else if err != nil {
		return be, errors.Wrap(err, "container.GetProperties")
	}

	return be, nil
}

// SetListMaxItems sets the number of list items to load per request.
func (be *Backend) SetListMaxItems(i int) {
	be.listMaxItems = i
}

// IsNotExist returns true if the error is caused by a not existing file.
func (be *Backend) IsNotExist(err error) bool {
	return bloberror.HasCode(err, bloberror.BlobNotFound)
}

func (be *Backend) IsPermanentError(err error) bool {
	if be.IsNotExist(err) {
		return true
	}

	var aerr *azcore.ResponseError
	if errors.As(err, &aerr) {
		if aerr.StatusCode == http.StatusRequestedRangeNotSatisfiable || aerr.StatusCode == http.StatusUnauthorized || aerr.StatusCode == http.StatusForbidden {
			return true
		}
	}
	return false
}

func (be *Backend) Properties() backend.Properties {
	return backend.Properties{
		Connections:      be.connections,
		HasAtomicReplace: true,
	}
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *Backend) Hasher() hash.Hash {
	return md5.New()
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.prefix
}

// useAccessTier determines whether to apply the configured access tier to a given file.
// For archive access tier, only data files are stored using that class; metadata
// must remain instantly accessible.
func (be *Backend) useAccessTier(h backend.Handle) bool {
	notArchiveClass := !strings.EqualFold(be.cfg.AccessTier, "archive")
	isDataFile := h.Type == backend.PackFile && !h.IsMetadata
	return isDataFile || notArchiveClass
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	objName := be.Filename(h)

	debug.Log("InsertObject(%v, %v)", be.cfg.AccountName, objName)

	var accessTier blob.AccessTier
	if be.useAccessTier(h) {
		accessTier = be.accessTier
	}

	var err error
	if rd.Length() < saveLargeSize {
		// if it's smaller than 256miB, then just create the file directly from the reader
		err = be.saveSmall(ctx, objName, rd, accessTier)
	} else {
		// otherwise use the more complicated method
		err = be.saveLarge(ctx, objName, rd, accessTier)
	}

	return err
}

func (be *Backend) saveSmall(ctx context.Context, objName string, rd backend.RewindReader, accessTier blob.AccessTier) error {
	blockBlobClient := be.container.NewBlockBlobClient(objName)

	// upload it as a new "block", use the base64 hash for the ID
	id := base64.StdEncoding.EncodeToString(rd.Hash())

	buf := make([]byte, rd.Length())
	_, err := io.ReadFull(rd, buf)
	if err != nil {
		return errors.Wrap(err, "ReadFull")
	}

	reader := bytes.NewReader(buf)
	_, err = blockBlobClient.StageBlock(ctx, id, streaming.NopCloser(reader), &blockblob.StageBlockOptions{
		TransactionalValidation: blob.TransferValidationTypeMD5(rd.Hash()),
	})
	if err != nil {
		return errors.Wrap(err, "StageBlock")
	}

	blocks := []string{id}
	_, err = blockBlobClient.CommitBlockList(ctx, blocks, &blockblob.CommitBlockListOptions{
		Tier: &accessTier,
	})
	return errors.Wrap(err, "CommitBlockList")
}

func (be *Backend) saveLarge(ctx context.Context, objName string, rd backend.RewindReader, accessTier blob.AccessTier) error {
	blockBlobClient := be.container.NewBlockBlobClient(objName)

	buf := make([]byte, 100*1024*1024)
	blocks := []string{}
	uploadedBytes := 0

	for {
		n, err := io.ReadFull(rd, buf)
		if err == io.ErrUnexpectedEOF {
			err = nil
		}

		if err == io.EOF {
			// end of file reached, no bytes have been read at all
			break
		}

		if err != nil {
			return errors.Wrap(err, "ReadFull")
		}

		buf = buf[:n]
		uploadedBytes += n

		// upload it as a new "block", use the base64 hash for the ID
		h := md5.Sum(buf)
		id := base64.StdEncoding.EncodeToString(h[:])

		reader := bytes.NewReader(buf)
		debug.Log("StageBlock %v with %d bytes", id, len(buf))
		_, err = blockBlobClient.StageBlock(ctx, id, streaming.NopCloser(reader), &blockblob.StageBlockOptions{
			TransactionalValidation: blob.TransferValidationTypeMD5(h[:]),
		})

		if err != nil {
			return errors.Wrap(err, "StageBlock")
		}

		blocks = append(blocks, id)
	}

	// sanity check
	if uploadedBytes != int(rd.Length()) {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", uploadedBytes, rd.Length())
	}

	_, err := blockBlobClient.CommitBlockList(ctx, blocks, &blockblob.CommitBlockListOptions{
		Tier: &accessTier,
	})

	debug.Log("uploaded %d parts: %v", len(blocks), blocks)
	return errors.Wrap(err, "CommitBlockList")
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return util.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *Backend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	objName := be.Filename(h)
	blockBlobClient := be.container.NewBlobClient(objName)

	resp, err := blockBlobClient.DownloadStream(ctx, &blob.DownloadStreamOptions{
		Range: azblob.HTTPRange{
			Offset: offset,
			Count:  int64(length),
		},
	})

	if err != nil {
		return nil, err
	}

	if length > 0 && (resp.ContentLength == nil || *resp.ContentLength != int64(length)) {
		_ = resp.Body.Close()
		return nil, &azcore.ResponseError{ErrorCode: "restic-file-too-short", StatusCode: http.StatusRequestedRangeNotSatisfiable}
	}

	return resp.Body, err
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	objName := be.Filename(h)
	blobClient := be.container.NewBlobClient(objName)

	props, err := blobClient.GetProperties(ctx, nil)

	if err != nil {
		return backend.FileInfo{}, errors.Wrap(err, "blob.GetProperties")
	}

	fi := backend.FileInfo{
		Size: *props.ContentLength,
		Name: h.Name,
	}
	return fi, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h backend.Handle) error {
	objName := be.Filename(h)
	blob := be.container.NewBlobClient(objName)

	_, err := blob.Delete(ctx, &azblob.DeleteBlobOptions{})

	if be.IsNotExist(err) {
		return nil
	}

	return errors.Wrap(err, "client.RemoveObject")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (be *Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	prefix, _ := be.Basedir(t)

	// make sure prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	maxI := int32(be.listMaxItems)

	opts := &azContainer.ListBlobsFlatOptions{
		MaxResults: &maxI,
		Prefix:     &prefix,
	}
	lister := be.container.NewListBlobsFlatPager(opts)

	for lister.More() {
		resp, err := lister.NextPage(ctx)

		if err != nil {
			return err
		}

		debug.Log("got %v objects", len(resp.Segment.BlobItems))

		for _, item := range resp.Segment.BlobItems {
			m := strings.TrimPrefix(*item.Name, prefix)
			if m == "" {
				continue
			}

			fi := backend.FileInfo{
				Name: path.Base(m),
				Size: *item.Properties.ContentLength,
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}

			err := fn(fi)
			if err != nil {
				return err
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}

		}
	}

	return ctx.Err()
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *Backend) Delete(ctx context.Context) error {
	return util.DefaultDelete(ctx, be)
}

// Close does nothing
func (be *Backend) Close() error { return nil }

// Warmup not implemented
func (be *Backend) Warmup(_ context.Context, _ []backend.Handle) ([]backend.Handle, error) {
	return []backend.Handle{}, nil
}
func (be *Backend) WarmupWait(_ context.Context, _ []backend.Handle) error { return nil }
