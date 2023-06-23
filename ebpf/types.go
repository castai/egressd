package ebpf

import (
	"fmt"
	"strings"

	"github.com/google/gopacket/layers"
)

type DNSEvent struct {
	Questions []layers.DNSQuestion
	Answers   []layers.DNSResourceRecord
}

func (e DNSEvent) String() string {
	var str strings.Builder
	for _, v := range e.Questions {
		str.WriteString("Questions:\n")
		str.WriteString(fmt.Sprintf("%s %s %s \n", v.Class, v.Type, string(v.Name)))
	}
	for _, v := range e.Answers {
		str.WriteString("Answers:\n")
		str.WriteString(fmt.Sprintf("%s %s %s [%s] [%s] \n", v.Class, v.Type, string(v.Name), v.IP, v.CNAME))
	}
	return str.String()
}
