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
package agentserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // Registers /debug/pprof endpoints in http.DefaultServeMux.
	"os"
	"strings"

	"github.com/uber/kraken/build-index/tagclient"
	"github.com/uber/kraken/core"
	"github.com/uber/kraken/lib/dockerdaemon"
	"github.com/uber/kraken/lib/middleware"
	"github.com/uber/kraken/lib/store"
	"github.com/uber/kraken/lib/torrent/scheduler"
	"github.com/uber/kraken/utils/handler"
	"github.com/uber/kraken/utils/httputil"

	"github.com/pressly/chi"
	"github.com/uber-go/tally"
)

// Config defines Server configuration.
type Config struct{}

// Server defines the agent HTTP server.
type Server struct {
	config    Config
	stats     tally.Scope
	cads      *store.CADownloadStore
	sched     scheduler.ReloadableScheduler
	tags      tagclient.Client
	dockerCli dockerdaemon.DockerClient
}

// New creates a new Server.
func New(
	config Config,
	stats tally.Scope,
	cads *store.CADownloadStore,
	sched scheduler.ReloadableScheduler,
	tags tagclient.Client,
	dockerCli dockerdaemon.DockerClient) *Server {

	stats = stats.Tagged(map[string]string{
		"module": "agentserver",
	})

	return &Server{config, stats, cads, sched, tags, dockerCli}
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.StatusCounter(s.stats))
	r.Use(middleware.LatencyTimer(s.stats))

	r.Get("/health", handler.Wrap(s.healthHandler))

	r.Get("/tags/{tag}", handler.Wrap(s.getTagHandler))

	// GET /v2/<name>/blobs/<digest>
	r.Get("/namespace/{namespace}/blobs/{digest}", handler.Wrap(s.downloadBlobHandler))

	r.Delete("/blobs/{digest}", handler.Wrap(s.deleteBlobHandler))

	// Preheat/preload endpoints.
	r.Get("/preload/tags/{tag}", handler.Wrap(s.preloadTagHandler))

	// Dangerous endpoint for running experiments.
	r.Patch("/x/config/scheduler", handler.Wrap(s.patchSchedulerConfigHandler))

	r.Get("/x/blacklist", handler.Wrap(s.getBlacklistHandler))

	// Serves /debug/pprof endpoints.
	r.Mount("/", http.DefaultServeMux)

	return r
}

// getTagHandler proxies get tag requests to the build-index.
func (s *Server) getTagHandler(w http.ResponseWriter, r *http.Request) error {
	tag, err := httputil.ParseParam(r, "tag")
	if err != nil {
		return err
	}
	d, err := s.tags.Get(tag)
	if err != nil {
		if err == tagclient.ErrTagNotFound {
			return handler.ErrorStatus(http.StatusNotFound)
		}
		return handler.Errorf("get tag: %s", err)
	}
	io.WriteString(w, d.String())
	return nil
}

// downloadBlobHandler downloads a blob through p2p.
// 在方法中调用了 metaData 接口
func (s *Server) downloadBlobHandler(w http.ResponseWriter, r *http.Request) error {
	namespace, err := httputil.ParseParam(r, "namespace")
	if err != nil {
		return err
	}
	d, err := parseDigest(r)
	if err != nil {
		return err
	}
	// 查询缓存是否已经存在下载文件
	f, err := s.cads.Cache().GetFileReader(d.Hex())
	if err != nil {
		if os.IsNotExist(err) || s.cads.InDownloadError(err) {
			// 如果本地没有或者查询失败，则调用了 traker metaData 接口 获取
			if err := s.sched.Download(namespace, d); err != nil {
				if err == scheduler.ErrTorrentNotFound {
					return handler.ErrorStatus(http.StatusNotFound)
				}
				return handler.Errorf("download torrent: %s", err)
			}
			// 获取下载文件
			f, err = s.cads.Cache().GetFileReader(d.Hex())
			if err != nil {
				return handler.Errorf("store: %s", err)
			}
		} else {
			return handler.Errorf("store: %s", err)
		}
	}
	// todo 整个文件下载完成后才返回？
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("copy file: %s", err)
	}
	return nil
}

func (s *Server) deleteBlobHandler(w http.ResponseWriter, r *http.Request) error {
	d, err := parseDigest(r)
	if err != nil {
		return err
	}
	if err := s.sched.RemoveTorrent(d); err != nil {
		return handler.Errorf("remove torrent: %s", err)
	}
	return nil
}

// preloadTagHandler triggers docker daemon to download specified docker image.
func (s *Server) preloadTagHandler(w http.ResponseWriter, r *http.Request) error {
	tag, err := httputil.ParseParam(r, "tag")
	if err != nil {
		return err
	}
	parts := strings.Split(tag, ":")
	if len(parts) != 2 {
		return handler.Errorf("failed to parse docker image tag")
	}
	repo, tag := parts[0], parts[1]
	if err := s.dockerCli.PullImage(
		context.Background(), repo, tag); err != nil {

		return handler.Errorf("trigger docker pull: %s", err)
	}
	return nil
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) error {
	// 验证调度程序事件循环是否正在运行并未阻塞。
	if err := s.sched.Probe(); err != nil {
		return handler.Errorf("probe torrent client: %s", err)
	}
	fmt.Fprintln(w, "OK")
	return nil
}

// patchSchedulerConfigHandler restarts the agent torrent scheduler with
// the config in request body.
func (s *Server) patchSchedulerConfigHandler(w http.ResponseWriter, r *http.Request) error {
	defer r.Body.Close()
	var config scheduler.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		return handler.Errorf("json decode: %s", err).Status(http.StatusBadRequest)
	}
	s.sched.Reload(config)
	return nil
}

func (s *Server) getBlacklistHandler(w http.ResponseWriter, r *http.Request) error {
	blacklist, err := s.sched.BlacklistSnapshot()
	if err != nil {
		return handler.Errorf("blacklist snapshot: %s", err)
	}
	if err := json.NewEncoder(w).Encode(&blacklist); err != nil {
		return handler.Errorf("json encode: %s", err)
	}
	return nil
}

// 解析 sha256 签名
func parseDigest(r *http.Request) (core.Digest, error) {
	raw, err := httputil.ParseParam(r, "digest")
	if err != nil {
		return core.Digest{}, err
	}
	// TODO(codyg): Accept only a fully formed digest.
	d, err := core.NewSHA256DigestFromHex(raw)
	if err != nil {
		d, err = core.ParseSHA256Digest(raw)
		if err != nil {
			return core.Digest{}, handler.Errorf("parse digest: %s", err).Status(http.StatusBadRequest)
		}
	}
	return d, nil
}
