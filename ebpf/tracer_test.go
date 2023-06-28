package ebpf

import (
	"fmt"
	"testing"
)

func TestTracer(t *testing.T) {
	payload := []byte{69, 0, 0, 80, 106, 126, 64, 0, 62, 17, 36, 91, 192, 168, 65, 7, 172, 17, 0, 3, 0, 53, 224, 37, 0, 60, 174, 17, 191, 43, 129, 128, 0, 1, 0, 1, 0, 0, 0, 0, 6, 103, 111, 111, 103, 108, 101, 2, 108, 116, 0, 0, 1, 0, 1, 6, 103, 111, 111, 103, 108, 101, 2, 108, 116, 0, 0, 1, 0, 1, 0, 0, 1, 121, 0, 4, 172, 217, 21, 163}
	event, err := parseEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(event.String())
}
