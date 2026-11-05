// SPDX-License-Identifier: MIT

// Package maruaddon is the Go SDK for Maru NAS addon package backends.
//
// An addon package may ship a backend: a static Linux executable that systemd runs as a dynamic user when the
// package is installed. The dashboard (maru) talks to it over HTTP/1.1 on a Unix socket:
//
//   - GET /v1/data?<query> is answered by [Handler.Data].
//   - POST /v1/actions/{name} is answered by the matching function in [Handler.Actions].
//
// Every request carries the dashboard user ([Request.User], [Request.Admin]), the display language
// ([Request.Lang]) and a call token that lets the backend call OMV through maru while it handles that request
// ([Request.Host]). maru only lets a backend call the OMV methods its manifest declares, and the token stops
// working when the request ends, so a backend cannot call OMV on its own.
//
// A minimal backend:
//
//	func main() {
//		err := maruaddon.Serve(maruaddon.Handler{
//			Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
//				return map[string]string{"hello": r.User}, nil
//			},
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//	}
//
// [Serve] takes the listening socket from systemd socket activation, or from MARU_ADDON_SOCKET in development, and
// stops gracefully on SIGTERM or SIGINT.
//
// Handler results become the response:
//
//   - A data function returning (v, nil) answers 200 with v as JSON (a nil v is JSON null).
//   - An action returning (nil, nil) answers 204; returning (v, nil) answers 200 with v as JSON.
//   - An [*Error] (also when wrapped) with status 400, 404, 409, 422 or 429 and a valid code answers that status
//     with {"code", "message"}; maru passes it to the addon's frontend.
//   - Any other error answers 500 {"code":"internal","message":"internal error"} and is written to
//     [Handler.ErrorLog] (standard error, and so the systemd journal, by default).
//
// The full protocol is described in the Maru NAS documentation:
// https://github.com/steamb23/maru-nas/blob/main/docs/en/addon-backend.md
package maruaddon
