package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestIncExportedEvents(t *testing.T) {
	r := require.New(t)

	IncExportedEvents(1)
	IncExportedEvents(1)

	problems, err := testutil.CollectAndLint(exportedEventsTotal)
	r.NoError(err)
	r.Empty(problems)

	expected := `# HELP egressd_exported_events_total Counter for tracking exported events rate
# TYPE egressd_exported_events_total counter
egressd_exported_events_total 2
`
	r.NoError(testutil.CollectAndCompare(exportedEventsTotal, strings.NewReader(expected)))
}

func TestIncDroppedEvents(t *testing.T) {
	r := require.New(t)

	IncDroppedEvents()

	problems, err := testutil.CollectAndLint(droppedEventsTotal)
	r.NoError(err)
	r.Empty(problems)

	expected := `# HELP egressd_dropped_events_total Counter for tracking dropped events rate
	# TYPE egressd_dropped_events_total counter
	egressd_dropped_events_total 1
`
	r.NoError(testutil.CollectAndCompare(droppedEventsTotal, strings.NewReader(expected)))
}

func TestSetConntrackEntriesCount(t *testing.T) {
	r := require.New(t)

	SetConntrackEntriesCount(10)

	problems, err := testutil.CollectAndLint(conntrackEntriesCount)
	r.NoError(err)
	r.Empty(problems)

	expected := `# HELP egressd_conntrack_entries Gauge for conntrack entries count
# TYPE egressd_conntrack_entries gauge
egressd_conntrack_entries 10
`
	r.NoError(testutil.CollectAndCompare(conntrackEntriesCount, strings.NewReader(expected)))
}

func TestSetConntrackActiveEntriesCount(t *testing.T) {
	r := require.New(t)

	SetConntrackActiveEntriesCount(15)

	problems, err := testutil.CollectAndLint(conntrackActiveEntriesCount)
	r.NoError(err)
	r.Empty(problems)

	expected := `# HELP egressd_conntrack_entries_active Gauge for conntrack active entries count
# TYPE egressd_conntrack_entries_active gauge
egressd_conntrack_entries_active 15
`
	r.NoError(testutil.CollectAndCompare(conntrackActiveEntriesCount, strings.NewReader(expected)))
}
