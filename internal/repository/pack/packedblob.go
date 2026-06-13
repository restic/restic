package pack

import "github.com/restic/restic/internal/restic"

// PackedBlob is one index entry for a blob in a pack (may be duplicate across indexes).
type PackedBlob struct {
	Pack restic.ID
	Blob Blob
}

func (pb *PackedBlob) PackID() restic.ID { return pb.Pack }

func (pb *PackedBlob) Handle() restic.BlobHandle { return pb.Blob.BlobHandle }

func (pb *PackedBlob) CiphertextLength() uint { return pb.Blob.Length }

func (pb *PackedBlob) PlaintextLength() uint { return pb.Blob.DataLength() }

func (pb *PackedBlob) IsCompressed() bool { return pb.Blob.IsCompressed() }
