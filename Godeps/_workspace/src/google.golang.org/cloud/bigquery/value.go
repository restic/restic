// Copyright 2015 Google Inc. All Rights Reserved.
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

package bigquery

// Value stores the contents of a single cell from a BigQuery result.
type Value interface{}

// ValueLoader stores a slice of Values representing a result row from a Read operation.
// See Iterator.Get for more information.
type ValueLoader interface {
	Load(v []Value) error
}

// ValueList converts a []Value to implement ValueLoader.
type ValueList []Value

// Load stores a sequence of values in a ValueList.
func (vs *ValueList) Load(v []Value) error {
	*vs = append(*vs, v...)
	return nil
}
