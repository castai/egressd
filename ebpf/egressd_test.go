package ebpf

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestEgressd(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	prog, err := NewEgressd()
	r.NoError(err)

	fmt.Println("reading dns")
	r.NoError(prog.Start(ctx))
}
