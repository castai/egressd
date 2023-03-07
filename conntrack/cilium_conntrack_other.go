//go:build !linux

package conntrack

import (
	"errors"
)

func listRecords(maps []interface{}, clockSource ClockSource, filter EntriesFilter) ([]*Entry, error) {
	return nil, errors.New("not implemented")
}

func initMaps() []interface{} {
	return nil
}

func bpfMapsExist() bool {
	return false
}
