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
package cmd

import (
	"github.com/uber/kraken/core"
	"github.com/uber/kraken/lib/backend"
	"github.com/uber/kraken/lib/blobrefresh"
	"github.com/uber/kraken/lib/hashring"
	"github.com/uber/kraken/lib/healthcheck"
	"github.com/uber/kraken/lib/hostlist"
	"github.com/uber/kraken/lib/metainfogen"
	"github.com/uber/kraken/lib/persistedretry"
	"github.com/uber/kraken/lib/store"
	"github.com/uber/kraken/lib/torrent/networkevent"
	"github.com/uber/kraken/lib/torrent/scheduler"
	"github.com/uber/kraken/localdb"
	"github.com/uber/kraken/metrics"
	"github.com/uber/kraken/nginx"
	"github.com/uber/kraken/origin/blobserver"
	"github.com/uber/kraken/utils/httputil"

	"go.uber.org/zap"
)

// Config defines origin server configuration.
// TODO(evelynl94): consolidate cluster and hashring.
type Config struct {
	Verbose       bool
	ZapLogging    zap.Config               `yaml:"zap"`
	// 集群地址配置，可配置静态地址列表和DNS
	Cluster       hostlist.Config          `yaml:"cluster"`
	// hash ring 配置，指定一个blob的最大副本数量， 和地址列表/健康情况刷新间隔
	HashRing      hashring.Config          `yaml:"hashring"`
	// 集群节点健康检查方法， 因为 origin的集群一般比较小，所以集群节点间会互相检查 （Active Health Check）
	HealthCheck   healthcheck.FilterConfig `yaml:"healthcheck"`
	BlobServer    blobserver.Config        `yaml:"blobserver"`
	CAStore       store.CAStoreConfig      `yaml:"castore"`
	// scheduler 配置
	Scheduler     scheduler.Config         `yaml:"scheduler"`
	NetworkEvent  networkevent.Config      `yaml:"network_event"`
	PeerIDFactory core.PeerIDFactory       `yaml:"peer_id_factory"`
	// 监控项配置
	Metrics       metrics.Config           `yaml:"metrics"`
	MetaInfoGen   metainfogen.Config       `yaml:"metainfogen"`
	// 后端存储配置
	Backends      []backend.Config         `yaml:"backends"`
	// 后端存储用户认证
	Auth          backend.AuthConfig       `yaml:"auth"`
	BlobRefresh   blobrefresh.Config       `yaml:"blobrefresh"`
	LocalDB       localdb.Config           `yaml:"localdb"`
	WriteBack     persistedretry.Config    `yaml:"writeback"`
	// nginx 配置
	Nginx         nginx.Config             `yaml:"nginx"`
	TLS           httputil.TLSConfig       `yaml:"tls"`
}
