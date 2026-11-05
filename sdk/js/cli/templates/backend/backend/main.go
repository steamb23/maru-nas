// The package's backend. During development you start it by hand; once the package is installed on a dashboard with
// addon backends turned on (a later version), systemd starts it by socket activation. The dashboard never runs it
// itself: it forwards the frontend's app.data() and app.action() to the backend's socket. See README.md.
package main

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

func main() {
	// requests counts the data requests since the backend started. Every dashboard user talks to the same backend,
	// so they all see the same count. Keep lasting data in maruaddon.StateDir() instead.
	var requests atomic.Int64

	err := maruaddon.Serve(maruaddon.Handler{
		// app.data() in the frontend: GET /v1/data. The result is sent to the frame as JSON.
		Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
			// Only the OMV methods listed in ../addon/manifest.json (rpc.read) can be called.
			var info struct {
				Hostname string `json:"hostname"`
			}
			if err := r.Host.RPCInto(ctx, "System", "getInformation", nil, &info); err != nil {
				return nil, err
			}
			return map[string]any{"hostname": info.Hostname, "requests": requests.Add(1)}, nil
		},
		// app.action('reset') in the frontend: POST /v1/actions/reset. Actions must be declared in the manifest.
		Actions: map[string]maruaddon.HandlerFunc{
			"reset": func(ctx context.Context, r *maruaddon.Request) (any, error) {
				requests.Store(0)
				return nil, nil // 204, no result
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
