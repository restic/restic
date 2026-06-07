package repository

import (
	"github.com/restic/chunker"
	"github.com/restic/restic/internal/restic"
)

type baseChunker struct {
	bc  *chunker.BaseChunker
	pol chunker.Pol
}

func (c *baseChunker) Reset() {
	c.bc.Reset(c.pol)
}

func (c *baseChunker) NextSplitPoint(buf []byte) int {
	return c.bc.NextSplitPoint(buf)
}

type chunkerFactory struct {
	pol chunker.Pol
}

func newChunkerFactory(pol chunker.Pol) *chunkerFactory {
	return &chunkerFactory{pol: pol}
}

func (f *chunkerFactory) NewChunker() restic.Chunker {
	return &baseChunker{bc: chunker.NewBase(f.pol), pol: f.pol}
}

func (f *chunkerFactory) MaxChunkSize() int {
	return chunker.MaxSize
}

func (r *Repository) ChunkerFactory() restic.ChunkerFactory {
	return newChunkerFactory(r.Config().ChunkerPolynomial)
}
