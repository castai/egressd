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

func (c *ciliumClient) ListEntries(filter EntriesFilter) ([]Entry, error) {
	records, err := listRecords(c.maps, filter)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (c *ciliumClient) Close() error {
	return nil
}
