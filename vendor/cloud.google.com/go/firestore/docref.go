// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"errors"
	"reflect"
	"sort"

	"golang.org/x/net/context"
	"google.golang.org/api/iterator"

	vkit "cloud.google.com/go/firestore/apiv1beta1"
	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"
)

var errNilDocRef = errors.New("firestore: nil DocumentRef")

// A DocumentRef is a reference to a Firestore document.
type DocumentRef struct {
	// The CollectionRef that this document is a part of. Never nil.
	Parent *CollectionRef

	// The full resource path of the document: "projects/P/databases/D/documents..."
	Path string

	// The ID of the document: the last component of the resource path.
	ID string
}

func newDocRef(parent *CollectionRef, id string) *DocumentRef {
	return &DocumentRef{
		Parent: parent,
		ID:     id,
		Path:   parent.Path + "/" + id,
	}
}

func (d1 *DocumentRef) equal(d2 *DocumentRef) bool {
	if d1 == nil || d2 == nil {
		return d1 == d2
	}
	return d1.Parent.equal(d2.Parent) && d1.Path == d2.Path && d1.ID == d2.ID
}

// Collection returns a reference to sub-collection of this document.
func (d *DocumentRef) Collection(id string) *CollectionRef {
	return newCollRefWithParent(d.Parent.c, d, id)
}

// Get retrieves the document. It returns an error if the document does not exist.
func (d *DocumentRef) Get(ctx context.Context) (*DocumentSnapshot, error) {
	if err := checkTransaction(ctx); err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errNilDocRef
	}
	doc, err := d.Parent.c.c.GetDocument(withResourceHeader(ctx, d.Parent.c.path()),
		&pb.GetDocumentRequest{Name: d.Path})
	// TODO(jba): verify that GetDocument returns NOT_FOUND.
	if err != nil {
		return nil, err
	}
	return newDocumentSnapshot(d, doc, d.Parent.c)
}

// Create creates the document with the given data.
// It returns an error if a document with the same ID already exists.
//
// The data argument can be a map with string keys, a struct, or a pointer to a
// struct. The map keys or exported struct fields become the fields of the firestore
// document.
// The values of data are converted to Firestore values as follows:
//
//   - bool converts to Bool.
//   - string converts to String.
//   - int, int8, int16, int32 and int64 convert to Integer.
//   - uint8, uint16 and uint32 convert to Integer. uint64 is disallowed,
//     because it can represent values that cannot be represented in an int64, which
//     is the underlying type of a Integer.
//   - float32 and float64 convert to Double.
//   - []byte converts to Bytes.
//   - time.Time converts to Timestamp.
//   - latlng.LatLng converts to GeoPoint. latlng is the package
//     "google.golang.org/genproto/googleapis/type/latlng".
//   - Slices convert to Array.
//   - Maps and structs convert to Map.
//   - nils of any type convert to Null.
//
// Pointers and interface{} are also permitted, and their elements processed
// recursively.
//
// Struct fields can have tags like those used by the encoding/json package. Tags
// begin with "firestore:" and are followed by "-", meaning "ignore this field," or
// an alternative name for the field. Following the name, these comma-separated
// options may be provided:
//
//   - omitempty: Do not encode this field if it is empty. A value is empty
//     if it is a zero value, or an array, slice or map of length zero.
//   - serverTimestamp: The field must be of type time.Time. When writing, if
//     the field has the zero value, the server will populate the stored document with
//     the time that the request is processed.
func (d *DocumentRef) Create(ctx context.Context, data interface{}) (*WriteResult, error) {
	ws, err := d.newReplaceWrites(data, nil, Exists(false))
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

// Set creates or overwrites the document with the given data. See DocumentRef.Create
// for the acceptable values of data. Without options, Set overwrites the document
// completely. Specify one of the Merge options to preserve an existing document's
// fields.
func (d *DocumentRef) Set(ctx context.Context, data interface{}, opts ...SetOption) (*WriteResult, error) {
	ws, err := d.newReplaceWrites(data, opts, nil)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

// Delete deletes the document. If the document doesn't exist, it does nothing
// and returns no error.
func (d *DocumentRef) Delete(ctx context.Context, preconds ...Precondition) (*WriteResult, error) {
	ws, err := d.newDeleteWrites(preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

func (d *DocumentRef) newReplaceWrites(data interface{}, opts []SetOption, p Precondition) ([]*pb.Write, error) {
	if d == nil {
		return nil, errNilDocRef
	}
	origFieldPaths, allPaths, err := processSetOptions(opts)
	if err != nil {
		return nil, err
	}
	isMerge := len(origFieldPaths) > 0 || allPaths // was some Merge option specified?
	doc, serverTimestampPaths, err := toProtoDocument(data)
	if err != nil {
		return nil, err
	}
	if len(origFieldPaths) > 0 {
		// Keep only data fields corresponding to the given field paths.
		doc.Fields = applyFieldPaths(doc.Fields, origFieldPaths, nil)
	}
	doc.Name = d.Path

	var fieldPaths []FieldPath
	if allPaths {
		// MergeAll was passed. Check that the data is a map, and extract its field paths.
		v := reflect.ValueOf(data)
		if v.Kind() != reflect.Map {
			return nil, errors.New("firestore: MergeAll can only be specified with map data")
		}
		fieldPaths = fieldPathsFromMap(v, nil)
	} else if len(origFieldPaths) > 0 {
		// Remove server timestamp paths that are not in the list of paths to merge.
		// Note: this is technically O(n^2), but it is unlikely that there is more
		// than one server timestamp path.
		serverTimestampPaths = removePathsIf(serverTimestampPaths, func(fp FieldPath) bool {
			return !fp.in(origFieldPaths)
		})
		// Remove server timestamp fields from fieldPaths. Those fields were removed
		// from the document by toProtoDocument, so they should not be in the update
		// mask.
		// Note: this is technically O(n^2), but it is unlikely that there is
		// more than one server timestamp path.
		fieldPaths = removePathsIf(origFieldPaths, func(fp FieldPath) bool {
			return fp.in(serverTimestampPaths)
		})
		// Check that all the remaining field paths in the merge option are in the document.
		for _, fp := range fieldPaths {
			if _, err := valueAtPath(fp, doc.Fields); err != nil {
				return nil, err
			}
		}
	}
	var pc *pb.Precondition
	if p != nil {
		pc, err = p.preconditionProto()
		if err != nil {
			return nil, err
		}
	}
	var w *pb.Write
	switch {
	case len(fieldPaths) > 0:
		// There are field paths, so we need an update mask.
		sfps := toServiceFieldPaths(fieldPaths)
		sort.Strings(sfps) // TODO(jba): make tests pass without this
		w = &pb.Write{
			Operation:       &pb.Write_Update{doc},
			UpdateMask:      &pb.DocumentMask{FieldPaths: sfps},
			CurrentDocument: pc,
		}
	case isMerge && pc != nil:
		// There were field paths, but they all got removed.
		// The write does nothing but enforce the precondition.
		w = &pb.Write{CurrentDocument: pc}
	case !isMerge && (pc != nil || doc.Fields != nil):
		// Set without merge, so no update mask.
		w = &pb.Write{
			Operation:       &pb.Write_Update{doc},
			CurrentDocument: pc,
		}
	}
	return d.writeWithTransform(w, serverTimestampPaths), nil
}

// Create a new map that contains only the field paths in fps.
func applyFieldPaths(fields map[string]*pb.Value, fps []FieldPath, root FieldPath) map[string]*pb.Value {
	r := map[string]*pb.Value{}
	for k, v := range fields {
		kpath := root.with(k)
		if kpath.in(fps) {
			r[k] = v
		} else if mv := v.GetMapValue(); mv != nil {
			if m2 := applyFieldPaths(mv.Fields, fps, kpath); m2 != nil {
				r[k] = &pb.Value{&pb.Value_MapValue{&pb.MapValue{m2}}}
			}
		}
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func fieldPathsFromMap(vmap reflect.Value, prefix FieldPath) []FieldPath {
	// vmap is a map and its keys are strings.
	// Each map key denotes a field; no splitting or escaping.
	var fps []FieldPath
	for _, k := range vmap.MapKeys() {
		v := vmap.MapIndex(k)
		fp := prefix.with(k.String())
		if vm := extractMap(v); vm.IsValid() {
			fps = append(fps, fieldPathsFromMap(vm, fp)...)
		} else if v.Interface() != ServerTimestamp {
			// ServerTimestamp fields do not go into the update mask.
			fps = append(fps, fp)
		}
	}
	return fps
}

func extractMap(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Map:
		return v
	case reflect.Interface:
		return extractMap(v.Elem())
	default:
		return reflect.Value{}
	}
}

// removePathsIf creates a new slice of FieldPaths that contains
// exactly those elements of fps for which pred returns false.
func removePathsIf(fps []FieldPath, pred func(FieldPath) bool) []FieldPath {
	var result []FieldPath
	for _, fp := range fps {
		if !pred(fp) {
			result = append(result, fp)
		}
	}
	return result
}

func (d *DocumentRef) newDeleteWrites(preconds []Precondition) ([]*pb.Write, error) {
	if d == nil {
		return nil, errNilDocRef
	}
	pc, err := processPreconditionsForDelete(preconds)
	if err != nil {
		return nil, err
	}
	return []*pb.Write{{
		Operation:       &pb.Write_Delete{d.Path},
		CurrentDocument: pc,
	}}, nil
}

func (d *DocumentRef) newUpdateMapWrites(data map[string]interface{}, preconds []Precondition) ([]*pb.Write, error) {
	// Collect all the (top-level) keys map; they will comprise the update mask.
	// Also, translate the map into a sequence of FieldPathUpdates.
	var fps []FieldPath
	var fpus []FieldPathUpdate
	for k, v := range data {
		fp, err := parseDotSeparatedString(k)
		if err != nil {
			return nil, err
		}
		fps = append(fps, fp)
		fpus = append(fpus, FieldPathUpdate{Path: fp, Value: v})
	}
	// Check that there are no duplicate field paths, and that no field
	// path is a prefix of another.
	if err := checkNoDupOrPrefix(fps); err != nil {
		return nil, err
	}
	// Re-create the map from the field paths and their corresponding values. A field path
	// with a Delete value will not appear in the map but it will appear in the
	// update mask, which will cause it to be deleted.
	m := createMapFromFieldPathUpdates(fpus)
	return d.newUpdateWrites(m, fps, preconds)
}

func (d *DocumentRef) newUpdateStructWrites(fieldPaths []string, data interface{}, preconds []Precondition) ([]*pb.Write, error) {
	if !isStructOrStructPtr(data) {
		return nil, errors.New("firestore: data is not struct or struct pointer")
	}
	fps, err := parseDotSeparatedStrings(fieldPaths)
	if err != nil {
		return nil, err
	}
	if err := checkNoDupOrPrefix(fps); err != nil {
		return nil, err
	}
	return d.newUpdateWrites(data, fps, preconds)
}

func (d *DocumentRef) newUpdatePathWrites(data []FieldPathUpdate, preconds []Precondition) ([]*pb.Write, error) {
	var fps []FieldPath
	for _, fpu := range data {
		if err := fpu.Path.validate(); err != nil {
			return nil, err
		}
		fps = append(fps, fpu.Path)
	}
	if err := checkNoDupOrPrefix(fps); err != nil {
		return nil, err
	}
	m := createMapFromFieldPathUpdates(data)
	return d.newUpdateWrites(m, fps, preconds)
}

// newUpdateWrites creates Write operations for an update.
func (d *DocumentRef) newUpdateWrites(data interface{}, fieldPaths []FieldPath, preconds []Precondition) ([]*pb.Write, error) {
	if len(fieldPaths) == 0 {
		return nil, errors.New("firestore: no paths to update")
	}
	if d == nil {
		return nil, errNilDocRef
	}
	pc, err := processPreconditionsForUpdate(preconds)
	if err != nil {
		return nil, err
	}
	doc, serverTimestampPaths, err := toProtoDocument(data)
	if err != nil {
		return nil, err
	}
	sfps := toServiceFieldPaths(fieldPaths)
	doc.Name = d.Path
	return d.writeWithTransform(&pb.Write{
		Operation:       &pb.Write_Update{doc},
		UpdateMask:      &pb.DocumentMask{FieldPaths: sfps},
		CurrentDocument: pc,
	}, serverTimestampPaths), nil
}

var requestTimeTransform = &pb.DocumentTransform_FieldTransform_SetToServerValue{
	pb.DocumentTransform_FieldTransform_REQUEST_TIME,
}

func (d *DocumentRef) writeWithTransform(w *pb.Write, serverTimestampFieldPaths []FieldPath) []*pb.Write {
	var ws []*pb.Write
	if w != nil {
		ws = append(ws, w)
	}
	if len(serverTimestampFieldPaths) > 0 {
		ws = append(ws, d.newTransform(serverTimestampFieldPaths))
	}
	return ws
}

func (d *DocumentRef) newTransform(serverTimestampFieldPaths []FieldPath) *pb.Write {
	sort.Sort(byPath(serverTimestampFieldPaths)) // TODO(jba): make tests pass without this
	var fts []*pb.DocumentTransform_FieldTransform
	for _, p := range serverTimestampFieldPaths {
		fts = append(fts, &pb.DocumentTransform_FieldTransform{
			FieldPath:     p.toServiceFieldPath(),
			TransformType: requestTimeTransform,
		})
	}
	return &pb.Write{
		Operation: &pb.Write_Transform{
			&pb.DocumentTransform{
				Document:        d.Path,
				FieldTransforms: fts,
				// TODO(jba): should the transform have the same preconditions as the write?
			},
		},
	}
}

type sentinel int

const (
	// Delete is used as a value in a call to UpdateMap to indicate that the
	// corresponding key should be deleted.
	Delete sentinel = iota

	// ServerTimestamp is used as a value in a call to UpdateMap to indicate that the
	// key's value should be set to the time at which the server processed
	// the request.
	ServerTimestamp
)

func (s sentinel) String() string {
	switch s {
	case Delete:
		return "Delete"
	case ServerTimestamp:
		return "ServerTimestamp"
	default:
		return "<?sentinel?>"
	}
}

// UpdateMap updates the document using the given data. Map keys replace the stored
// values, but other fields of the stored document are untouched.
// See DocumentRef.Create for acceptable map values.
//
// If a map key is a multi-element field path, like "a.b", then only key "b" of
// the map value at "a" is changed; the rest of the map is preserved.
// For example, if the stored data is
//     {"a": {"b": 1, "c": 2}}
// then
//     UpdateMap({"a": {"b": 3}}) => {"a": {"b": 3}}
// while
//     UpdateMap({"a.b": 3}) => {"a": {"b": 3, "c": 2}}
//
// To delete a key, specify it in the input with a value of firestore.Delete.
//
// Field paths expressed as map keys must not contain any of the runes "˜*/[]".
// Use UpdatePaths instead for such paths.
//
// UpdateMap returns an error if the document does not exist.
func (d *DocumentRef) UpdateMap(ctx context.Context, data map[string]interface{}, preconds ...Precondition) (*WriteResult, error) {
	ws, err := d.newUpdateMapWrites(data, preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

func isStructOrStructPtr(x interface{}) bool {
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Struct {
		return true
	}
	if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
		return true
	}
	return false
}

// UpdateStruct updates the given field paths of the stored document from the fields
// of data, which must be a struct or a pointer to a struct. Other fields of the
// stored document are untouched.
// See DocumentRef.Create for the acceptable values of the struct's fields.
//
// Each element of fieldPaths is a single field or a dot-separated sequence of
// fields, none of which contain the runes "˜*/[]".
//
// If an element of fieldPaths does not have a corresponding field in the struct,
// that key is deleted from the stored document.
//
// UpdateStruct returns an error if the document does not exist.
func (d *DocumentRef) UpdateStruct(ctx context.Context, fieldPaths []string, data interface{}, preconds ...Precondition) (*WriteResult, error) {
	ws, err := d.newUpdateStructWrites(fieldPaths, data, preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

// A FieldPathUpdate describes an update to a value referred to by a FieldPath.
// See DocumentRef.Create for acceptable values.
// To delete a field, specify firestore.Delete as the value.
type FieldPathUpdate struct {
	Path  FieldPath
	Value interface{}
}

// UpdatePaths updates the document using the given data. The values at the given
// field paths are replaced, but other fields of the stored document are untouched.
func (d *DocumentRef) UpdatePaths(ctx context.Context, data []FieldPathUpdate, preconds ...Precondition) (*WriteResult, error) {
	ws, err := d.newUpdatePathWrites(data, preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit(ctx, ws)
}

// Collections returns an interator over the immediate sub-collections of the document.
func (d *DocumentRef) Collections(ctx context.Context) *CollectionIterator {
	client := d.Parent.c
	it := &CollectionIterator{
		err:    checkTransaction(ctx),
		client: client,
		parent: d,
		it: client.c.ListCollectionIds(
			withResourceHeader(ctx, client.path()),
			&pb.ListCollectionIdsRequest{Parent: d.Path}),
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// CollectionIterator is an iterator over sub-collections of a document.
type CollectionIterator struct {
	client   *Client
	parent   *DocumentRef
	it       *vkit.StringIterator
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*CollectionRef
	err      error
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *CollectionIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *CollectionIterator) Next() (*CollectionRef, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *CollectionIterator) fetch(pageSize int, pageToken string) (string, error) {
	if it.err != nil {
		return "", it.err
	}
	return iterFetch(pageSize, pageToken, it.it.PageInfo(), func() error {
		id, err := it.it.Next()
		if err != nil {
			return err
		}
		var cr *CollectionRef
		if it.parent == nil {
			cr = newTopLevelCollRef(it.client, it.client.path(), id)
		} else {
			cr = newCollRefWithParent(it.client, it.parent, id)
		}
		it.items = append(it.items, cr)
		return nil
	})
}

// GetAll returns all the collections remaining from the iterator.
func (it *CollectionIterator) GetAll() ([]*CollectionRef, error) {
	var crs []*CollectionRef
	for {
		cr, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		crs = append(crs, cr)
	}
	return crs, nil
}

// Common fetch code for iterators that are backed by vkit iterators.
// TODO(jba): dedup with same function in logging/logadmin.
func iterFetch(pageSize int, pageToken string, pi *iterator.PageInfo, next func() error) (string, error) {
	pi.MaxSize = pageSize
	pi.Token = pageToken
	// Get one item, which will fill the buffer.
	if err := next(); err != nil {
		return "", err
	}
	// Collect the rest of the buffer.
	for pi.Remaining() > 0 {
		if err := next(); err != nil {
			return "", err
		}
	}
	return pi.Token, nil
}
