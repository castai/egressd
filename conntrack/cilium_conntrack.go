package conntrack

func NewCiliumClient() (Client, error) {
	maps := initMaps()
	return &ciliumClient{maps: maps}, nil
}

func CiliumAvailable() bool {
	return bpfMapsExist()
}

type ciliumClient struct {
	maps []interface{}
}

func (c *ciliumClient) ListEntries(filter EntriesFilter) ([]*Entry, error) {
	return listRecords(c.maps, filter)
}

func (c *ciliumClient) Close() error {
	return nil
}
