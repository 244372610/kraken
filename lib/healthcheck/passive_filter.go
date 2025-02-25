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
package healthcheck

import (
	"sync"
	"time"

	"github.com/andres-erbsen/clock"
	"github.com/uber/kraken/utils/stringset"
)

// PassiveFilter filters unhealthy hosts passively by tracking failed
// requests to hosts. Clients are responsible for marking failures from
// individual hosts, and PassiveFilter updates which hosts are unhealthy. It is
// recommended that clients only mark failures for network errors, not HTTP
// errors.
// PassiveFilter 通过Failed方法来完成失败的计数，不需要有 healthcheck.Checker 来主动发起检查
type PassiveFilter interface {
	Filter
	Failed(addr string)
}

type passiveFilter struct {
	sync.Mutex
	config    PassiveFilterConfig
	clk       clock.Clock
	unhealthy map[string]time.Time
	failures  map[string][]time.Time
}

// NewPassiveFilter creates a new PassiveFilter.
func NewPassiveFilter(config PassiveFilterConfig, clk clock.Clock) PassiveFilter {
	config.applyDefaults()
	return &passiveFilter{
		config:    config,
		clk:       clk,
		unhealthy: make(map[string]time.Time),
		failures:  make(map[string][]time.Time),
	}
}

// Run removes any unhealthy from addrs.
func (f *passiveFilter) Run(addrs stringset.Set) stringset.Set {
	f.Lock()
	defer f.Unlock()

	healthy := addrs.Copy()

	for addr, t := range f.unhealthy {
		// 如果最近一次探测不健康距离现在的时间超过了 FailTimeout ，则从不健康的列表中删除，否则从健康的列表中删除
		if f.clk.Now().Sub(t) > f.config.FailTimeout {
			delete(f.unhealthy, addr)
		} else {
			healthy.Remove(addr)
		}
	}

	return healthy
}

// Failed marks a request to addr as failed.
func (f *passiveFilter) Failed(addr string) {
	f.Lock()
	defer f.Unlock()

	now := f.clk.Now()

	failures := f.failures[addr]

	// Pop off the expired failures.
	// 将已经过期的失败时间戳从数据中删除
	for len(failures) > 0 {
		if now.Sub(failures[0]) > f.config.FailTimeout {
			failures = failures[1:]
			continue
		}
		break
	}

	// Add latest failure.
	failures = append(failures, now)

	// 如果在 FailTimeout 时间间隔内失败的次数 > f.config.Fails 则标记节点为不健康。并且记录不健康的时间
	if len(failures) >= f.config.Fails {
		f.unhealthy[addr] = now
	}
	// 更新新的列表
	f.failures[addr] = failures
}
