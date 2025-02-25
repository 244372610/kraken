// Copyright (c) 2016-2019 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package transfer

import (
	"github.com/uber/kraken/core"
	"github.com/uber/kraken/lib/store"
)

// ImageTransferer defines an interface that transfers images
// todo 这个接口很关键，他把对官方register的访问替换成了对Kraken的访问
// 比如将 GET /v2/<name>/blobs/<digest> 替换成 GET /namespace/<name>/blobs/<digest>
type ImageTransferer interface {
	Stat(namespace string, d core.Digest) (*core.BlobInfo, error)
	Download(namespace string, d core.Digest) (store.FileReader, error)
	Upload(namespace string, d core.Digest, blob store.FileReader) error

	GetTag(tag string) (core.Digest, error)
	PutTag(tag string, d core.Digest) error
	ListTags(prefix string) ([]string, error)
}
