package dockerengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"docker-lifecycle-bench/internal/campaign"
	"docker-lifecycle-bench/internal/model"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestContainerConfigPreservesDefaultAndPinsWholeProfile(t *testing.T) {
	e := &Engine{spec: campaign.Spec{Image: "probe:test", Container: campaign.Container{NetworkMode: "none"}}}
	_, config := e.containerConfig(model.Request{RunID: "run", SampleID: 1})
	if config.CpusetCpus != "" || config.CgroupParent != "" || config.Runtime != "" {
		t.Fatal("default behavior changed")
	}
	if err := e.spec.SetSingleCPU(0); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []uint64{0, 1} {
		c, config := e.containerConfig(model.Request{RunID: "run", SampleID: sample})
		if config.CpusetCpus != "0" || config.CgroupParent != "dlbench.slice" || config.Runtime != "runc" || c.Labels[runIDLabel] != "run" {
			t.Fatal("canary and sample must share placement/ownership")
		}
	}
}

func TestPlacementCanaryCleanupAndFailure(t *testing.T) {
	for _, name := range []string{"pass", "verify-fails", "cancel", "wrong-host-config", "wrong-owner", "remove-fails"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			removed, called := false, false
			var createdConfig struct {
				Labels     map[string]string
				HostConfig containertypes.HostConfig
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					if err := json.NewDecoder(r.Body).Decode(&createdConfig); err != nil {
						t.Error(err)
					}
					w.WriteHeader(201)
					_, _ = w.Write([]byte(`{"Id":"abc"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/abc/start"):
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/containers/abc/json"):
					labels := createdConfig.Labels
					if name == "wrong-owner" {
						labels = map[string]string{managedLabel: "true", runIDLabel: "other"}
					}
					config := createdConfig.HostConfig
					if name == "wrong-host-config" {
						config.CpusetCpus = "0-7"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": "abc", "State": map[string]any{"Running": true, "Pid": 500}, "Config": map[string]any{"Labels": labels}, "HostConfig": config})
				case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/containers/abc"):
					if r.URL.Query().Get("force") != "1" {
						t.Errorf("cleanup must force remove: %s", r.URL)
					}
					if name == "remove-fails" {
						http.Error(w, `{"message":"cleanup failure"}`, 500)
						return
					}
					removed = true
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected Docker request %s %s", r.Method, r.URL)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithAPIVersion("1.53"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			spec := campaign.Spec{Image: "probe:test", Container: campaign.Container{NetworkMode: "none"}}
			if err := spec.SetSingleCPU(16); err != nil {
				t.Fatal(err)
			}
			e := &Engine{client: cli, spec: spec}
			err = e.PlacementCanary(ctx, "run", func(pid int, id string) error {
				called = true
				if pid != 500 || id != "abc" {
					t.Errorf("wrong canary: %d %s", pid, id)
				}
				if name == "cancel" {
					cancel()
					return ctx.Err()
				}
				if name == "verify-fails" {
					return errors.New("unbound shim")
				}
				return nil
			})
			if (err == nil) != (name == "pass") {
				t.Errorf("err=%v", err)
			}
			if removed != (name != "wrong-owner" && name != "remove-fails") {
				t.Errorf("removed=%v", removed)
			}
			if called == (name == "wrong-host-config") {
				t.Errorf("called=%v", called)
			}
		})
	}
}
