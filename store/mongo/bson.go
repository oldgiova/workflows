// Copyright 2021 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package mongo

import (
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

var (
	arrFace  []interface{}
	tArrFace = reflect.TypeOf(arrFace)
	tMap     = reflect.TypeOf(map[string]interface{}{})
)

func init() {
	// Use JSON defaults for decoding embedded documents and arrays
	defaultRegistry := bson.NewRegistryBuilder().
		RegisterTypeMapEntry(bsontype.Array, tArrFace).
		RegisterTypeMapEntry(bsontype.EmbeddedDocument, tMap)
	bson.DefaultRegistry = defaultRegistry.Build()
}
