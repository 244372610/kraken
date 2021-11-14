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

	"github.com/uber/kraken/utils/stringset"
)

// state tracks the health status of a set of hosts. In particular, it tracks
// consecutive passes or fails which cause hosts to transition between healthy
// and unhealthy.
//
// state is thread-safe.
// 追踪 host 集合的健康状态
type state struct {
	sync.Mutex
	config  FilterConfig
	all     stringset.Set
	healthy stringset.Set
	trend   map[string]int
}

func newState(config FilterConfig) *state {
	return &state{
		config:  config,
		all:     stringset.New(),
		healthy: stringset.New(),
		trend:   make(map[string]int),
	}
}

// sync sets the current state to addrs. New entries are initialized as healthy,
// while existing entries not found in addrs are removed from s.
// 设置 addrs， 对于没有在state中的条目设置成 healthy， 之前存在但是在 addrs 中没有找到的要删除掉
func (s *state) sync(addrs stringset.Set) {
	s.Lock()
	defer s.Unlock()

	for addr := range addrs {
		// 如果之前不存在，则加入到集合中
		if !s.all.Has(addr) {
			s.all.Add(addr)
			s.healthy.Add(addr)
		}
	}

	for addr := range s.healthy {
		// 在 addrs 中没有找到则从healthy集合中删除
		if !addrs.Has(addr) {
			s.healthy.Remove(addr)
			delete(s.trend, addr)
		}
	}
}

// failed marks addr as failed.
func (s *state) failed(addr string) {
	s.Lock()
	defer s.Unlock()

	s.trend[addr] = max(min(s.trend[addr]-1, -1), -s.config.Fails)
	// 如果失败的次数达到配置的失败伐值，则从healthy集合中删除对应节点
	if s.trend[addr] == -s.config.Fails {
		s.healthy.Remove(addr)
	}
}

// passed marks addr as passed.
func (s *state) passed(addr string) {
	s.Lock()
	defer s.Unlock()

	s.trend[addr] = min(max(s.trend[addr]+1, 1), s.config.Passes)
	// 如果连续成功的次数达到配置的成功伐值，则加入到healthy集合中
	if s.trend[addr] == s.config.Passes {
		s.healthy.Add(addr)
	}
}

// getHealthy returns the current healthy hosts.
func (s *state) getHealthy() stringset.Set {
	s.Lock()
	defer s.Unlock()

	return s.healthy.Copy()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
