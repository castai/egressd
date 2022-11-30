package conntrack

import "inet.af/netaddr"

func NewCiliumClient() (Client, error) {
	maps := initMaps()
	ctMaps := make([]interface{}, len(maps))
	for i, m := range maps {
		ctMaps[i] = m
	}
	return &ciliumClient{maps: ctMaps}, nil
}

type ciliumClient struct {
	maps []interface{}
}

func (c *ciliumClient) ListEntries() (map[netaddr.IP][]Entry, error) {
	records, err := listRecords(c.maps)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (c *ciliumClient) Close() error {
	return nil
}
