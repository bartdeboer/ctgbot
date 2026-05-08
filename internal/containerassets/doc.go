// Package containerassets exists as a compatibility shim for older deployed
// ctgbot binaries whose upgrade flow still runs:
//
//	go generate ./internal/containerassets
//
// The canonical embedded runtime image asset package now lives at
// internal/runtime/imageassets.
package containerassets

//go:generate go run ../../cmd/pack
