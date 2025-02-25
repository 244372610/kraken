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
package metainfogen

import (
	"fmt"

	"github.com/uber/kraken/core"
	"github.com/uber/kraken/lib/store"
	"github.com/uber/kraken/lib/store/metadata"
)

// Generator wraps static piece length configuration in order to deterministically
// generate metainfo.
// 生成分片元数据信息
type Generator struct {
	pieceLengthConfig *pieceLengthConfig
	cas               *store.CAStore
}

// New creates a new Generator.
func New(config Config, cas *store.CAStore) (*Generator, error) {
	plConfig, err := newPieceLengthConfig(config.PieceLengths)
	if err != nil {
		return nil, fmt.Errorf("piece length config: %s", err)
	}
	return &Generator{plConfig, cas}, nil
}

// Generate generates metainfo for the blob of d and writes it to disk.
// 生成 metainfo 并写到磁盘
func (g *Generator) Generate(d core.Digest) error {
	// 根据 digest 获取文件
	info, err := g.cas.GetCacheFileStat(d.Hex())
	if err != nil {
		return fmt.Errorf("cache stat: %s", err)
	}
	f, err := g.cas.GetCacheFileReader(d.Hex())
	if err != nil {
		return fmt.Errorf("get cache file: %s", err)
	}
	// 根据文件大小计算分片信息
	pieceLength := g.pieceLengthConfig.get(info.Size())
	mi, err := core.NewMetaInfo(d, f, pieceLength)
	if err != nil {
		return fmt.Errorf("create metainfo: %s", err)
	}
	if _, err := g.cas.SetCacheFileMetadata(d.Hex(), metadata.NewTorrentMeta(mi)); err != nil {
		return fmt.Errorf("set metainfo: %s", err)
	}
	return nil
}
