//go:build !linux

package conntrack

import (
	"errors"

	"inet.af/netaddr"
)

func listRecords(maps []interface{}, filter EntriesFilter) (map[netaddr.IP][]Entry, error) {
	return nil, errors.New("not implemented")
}

func initMaps() []interface{} {
	return nil
}
