package conntrack

import (
	"github.com/sirupsen/logrus"
)

func NewCiliumClient(log logrus.FieldLogger, clockSource ClockSource) (Client, error) {
	maps := initMaps()
	return &ciliumClient{
		log:         log,
		maps:        maps,
		clockSource: clockSource,
	}, nil
}

func CiliumAvailable() bool {
	return bpfMapsExist()
}

type ciliumClient struct {
	log         logrus.FieldLogger
	maps        []interface{}
	clockSource ClockSource
}

func (c *ciliumClient) ListEntries(filter EntriesFilter) ([]*Entry, error) {
	return listRecords(c.maps, c.clockSource, filter)
}

func (c *ciliumClient) Close() error {
	return nil
}
