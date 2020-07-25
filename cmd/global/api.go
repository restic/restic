package global

import (
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

const (
	DataBlob = restic.DataBlob
	TreeBlob = restic.TreeBlob
)

// types
type Repository = restic.Repository
type Node = restic.Node
type Tree = restic.Tree
type TagList = restic.TagList
type Cache = cache.Cache
type Options = options.Options

// functions
var FindLatestSnapshot = restic.FindLatestSnapshot
var LoadSnapshot = restic.LoadSnapshot
var NewBlobBuffer = restic.NewBlobBuffer
var CiphertextLength = restic.CiphertextLength
var NewRepository = repository.New
var NewCache = cache.New
