// SPDX-License-Identifier: MIT

package maruaddon_test

import (
	"context"
	"log"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

// A backend with a data function and one action. The manifest declares "backend": {"network": []},
// "rpc": {"read": ["Network.getGeneralSettings"]} and "actions": [{"name": "hostname", "admin": true}].
func Example() {
	err := maruaddon.Serve(maruaddon.Handler{
		// GET /v1/data?name=…
		Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
			name := r.Query.Get("name")
			if name == "" {
				name = r.User
			}
			return map[string]string{"greeting": "Hello, " + name, "lang": r.Lang}, nil
		},
		Actions: map[string]maruaddon.HandlerFunc{
			// POST /v1/actions/hostname with input {"short": true}
			"hostname": func(ctx context.Context, r *maruaddon.Request) (any, error) {
				var in struct {
					Short bool `json:"short"`
				}
				if err := r.DecodeInput(&in); err != nil {
					return nil, err // 400 bad_input
				}
				var dns struct {
					Hostname   string `json:"hostname"`
					Domainname string `json:"domainname"`
				}
				if err := r.Host.RPCInto(ctx, "Network", "getGeneralSettings", nil, &dns); err != nil {
					return nil, err // 500, logged to the journal
				}
				if in.Short || dns.Domainname == "" {
					return map[string]string{"hostname": dns.Hostname}, nil
				}
				return map[string]string{"hostname": dns.Hostname + "." + dns.Domainname}, nil
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
