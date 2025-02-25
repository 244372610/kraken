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
package hashring

import (
	"log"
	"sync"
	"time"

	"github.com/uber/kraken/core"
	"github.com/uber/kraken/lib/healthcheck"
	"github.com/uber/kraken/lib/hostlist"
	"github.com/uber/kraken/lib/hrw"
	"github.com/uber/kraken/utils/stringset"
)

const _defaultWeight = 100

// Watcher allows clients to watch the ring for changes. Whenever membership
// changes, each registered Watcher is notified with the latest hosts.
type Watcher interface {
	Notify(latest stringset.Set)
}

// Ring is a rendezvous hashing ring which calculates an ordered replica set
// of healthy addresses which own any given digest.
//
// Address membership within the ring is defined by a dynamic hostlist.List. On
// top of that, replica sets are filtered by the health status of their addresses.
// Membership and health status may be refreshed by using Monitor.
//
// Ring maintains the invariant that it is always non-empty and can always provide
// locations, although in some scenarios the provided locations are not guaranteed
// to be healthy (see Locations).
type Ring interface {
	// Locations 获取拥有给点 digest 的 host 列表
	Locations(d core.Digest) []string
	// Contains 对应的 addr 是否在 hash 环中
	Contains(addr string) bool
	// Monitor 根据 config 中的配置定时去刷新地址列表
	Monitor(stop <-chan struct{})
	// Refresh 获取最新的地址列表和健康的服务节点
	Refresh()
}

type ring struct {
	config  Config
	cluster hostlist.List
	filter  healthcheck.Filter

	mu      sync.RWMutex // Protects the following fields:
	// 服务节点列表
	addrs   stringset.Set
	hash    *hrw.RendezvousHash
	// 健康的服务节点列表
	healthy stringset.Set

	watchers []Watcher
}

// Option allows setting custom parameters for ring.
type Option func(*ring)

// WithWatcher adds a watcher to the ring. Can be used multiple times.
func WithWatcher(w Watcher) Option {
	return func(r *ring) { r.watchers = append(r.watchers, w) }
}

// New creates a new Ring whose members are defined by cluster.
func New(
	config Config, cluster hostlist.List, filter healthcheck.Filter, opts ...Option) Ring {

	config.applyDefaults()
	r := &ring{
		config:  config,
		cluster: cluster,
		filter:  filter,
	}
	for _, opt := range opts {
		opt(r)
	}
	r.Refresh()
	return r
}

// Locations returns an ordered replica set of healthy addresses which own d.
// If all addresses in the replica set are unhealthy, then returns the next
// healthy address. If all addresses in the ring are unhealthy, then returns
// the first address which owns d (regardless of health). As such, Locations
// always returns a non-empty list.
// 如果所有的节点都是不健康的状态，则返回第一个拥有 digest的节点（不管当前节点是否正常）
// 返回拥有digest的健康节点，如果所有拥有digest的节点都不健康，则返回next healthy address
func (r *ring) Locations(d core.Digest) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := r.hash.GetOrderedNodes(d.ShardID(), len(r.addrs))
	if len(nodes) != len(r.addrs) {
		// This should never happen.
		log.Fatal("invariant violation: ordered hash nodes not equal to cluster size")
	}
	// 如果所有的节点都是不健康的状态，则返回第一个拥有 digest的节点（不管当前节点是否正常）
	if len(r.healthy) == 0 {
		return []string{nodes[0].Label}
	}

	// 返回拥有digest的健康节点，如果所有拥有digest的节点都不健康，则返回next healthy address
	var locs []string
	for i := 0; i < len(nodes) && (len(locs) == 0 || i < r.config.MaxReplica); i++ {
		addr := nodes[i].Label
		if r.healthy.Has(addr) {
			locs = append(locs, addr)
		}
	}
	return locs
}

// Contains returns whether the ring contains addr.
// 判断 hashing 环是否有对应addr
func (r *ring) Contains(addr string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.addrs.Has(addr)
}

// Monitor refreshes the ring at the configured interval. Blocks until the
// provided stop channel is closed.
func (r *ring) Monitor(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-time.After(r.config.RefreshInterval):
			r.Refresh()
		}
	}
}

// Refresh updates the membership and health information of r.
func (r *ring) Refresh() {
	latest := r.cluster.Resolve()

	healthy := r.filter.Run(latest)

	hash := r.hash
	if !stringset.Equal(r.addrs, latest) {
		// 如果当前地址列表和最新的结果是否一致
		// Membership has changed -- update hash nodes.
		hash = hrw.NewRendezvousHash(hrw.Murmur3Hash, hrw.UInt64ToFloat64)
		for addr := range latest {
			hash.AddNode(addr, _defaultWeight)
		}
		// Notify watchers.
		// 通知 watchers
		for _, w := range r.watchers {
			w.Notify(latest.Copy())
		}
	}

	r.mu.Lock()
	r.addrs = latest
	r.hash = hash
	r.healthy = healthy
	r.mu.Unlock()
}
