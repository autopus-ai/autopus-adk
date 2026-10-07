package healthband_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

const contractCILine = `{"schema":"autopus.metric_observation.v1","series":"ci.failure_rate:CI","sample_key":"1042",` +
	`"observed_at":"2026-09-14T10:00:00Z","tiebreak":1042,"value":1,"attempt":2,"source":"gh"}`

func metricsPath(projectDir, name string) string {
	return filepath.Join(projectDir, ".autopus", "metrics", name)
}

func writeMetricsFile(t *testing.T, projectDir, name, body string) {
	t.Helper()
	path := metricsPath(projectDir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func readLines(t *testing.T, projectDir, name string) []string {
	t.Helper()
	data, err := os.ReadFile(metricsPath(projectDir, name))
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func lockStore(t *testing.T, projectDir string) *healthband.Locked {
	t.Helper()
	locked, err := healthband.NewStore(projectDir).Lock(context.Background(), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, locked.Unlock()) })
	return locked
}

func lineWith(field, value string) string {
	return strings.Replace(contractCILine, field, value, 1)
}

// S8: 3 valid observations, 1 truncated JSON line, 1 v2 schema line, and 1
// value-2 line give 3 observations and one count per class.
func TestReadObservations_SkipsAndCountsEachBadLineClass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile, strings.Join([]string{
		contractCILine,
		lineWith(`"sample_key":"1042"`, `"sample_key":"1043"`),
		`{"schema":"autopus.metric_observation.v1","series":"ci.fail`,
		lineWith(`metric_observation.v1`, `metric_observation.v2`),
		lineWith(`"value":1`, `"value":2`),
		"",
		lineWith(`"sample_key":"1042"`, `"sample_key":"1044"`),
	}, "\n")+"\n")

	observations, counts, err := healthband.NewStore(dir).ReadObservations(healthband.CIRunsFile)

	require.NoError(t, err)
	assert.Equal(t, []string{"1042", "1043", "1044"}, sampleKeys(observations))
	assert.Equal(t, healthband.ReadCounts{Malformed: 1, UnknownSchema: 1, InvalidValue: 1}, counts)
	assert.Equal(t, 3, counts.Skipped())
}

// Every field is validated: a line that decodes but breaks the contract is
// invalid_value, a line that is not a JSON object is malformed.
func TestReadObservations_ClassifiesContractViolations(t *testing.T) {
	t.Parallel()
	invalid := []string{
		strings.Replace(contractCILine, `,"value":1`, ``, 1),
		lineWith(`"value":1`, `"value":"1"`),
		lineWith(`"value":1`, `"value":0.5`),
		lineWith(`"value":1`, `"value":1e400`),
		lineWith(`"attempt":2`, `"attempt":0`),
		lineWith(`"series":"ci.failure_rate:CI"`, `"series":"ci.failure_rate:\u001b[2J"`),
		lineWith(`"series":"ci.failure_rate:CI"`, `"series":"cpu.load:CI"`),
		lineWith(`"sample_key":"1042"`, `"sample_key":""`),
		lineWith(`"sample_key":"1042"`, `"sample_key":"10 42"`),
		lineWith(`"observed_at":"2026-09-14T10:00:00Z"`, `"observed_at":"0001-01-01T00:00:00Z"`),
		lineWith(`"tiebreak":1042`, `"tiebreak":-1`),
		lineWith(`"source":"gh"`, `"source":"G H"`),
	}
	malformed := []string{`[1,2]`, `"text"`, `{"schema":`}
	unknown := []string{lineWith(`"schema":"autopus.metric_observation.v1"`, `"schema":5`), `{"series":"x"}`}
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile,
		strings.Join(append(append(append([]string{contractCILine}, invalid...), malformed...), unknown...), "\n"))

	observations, counts, err := healthband.NewStore(dir).ReadObservations(healthband.CIRunsFile)

	require.NoError(t, err)
	assert.Len(t, observations, 1)
	assert.Equal(t, healthband.ReadCounts{Malformed: len(malformed), UnknownSchema: len(unknown), InvalidValue: len(invalid)}, counts)
}

// REQ-01: the store directory is <projectDir>/.autopus/metrics, and every
// store file lies directly in it.
func TestNewStore_AddressesTheMetricsDirectory(t *testing.T) {
	t.Parallel()
	store := healthband.NewStore("/work/repo")

	assert.Equal(t, filepath.Join("/work/repo", ".autopus", "metrics"), store.Dir())
	assert.Equal(t, filepath.Join(store.Dir(), healthband.CIRunsFile), store.Path(healthband.CIRunsFile))
}

func TestReadObservations_MissingFileIsAnEmptyStore(t *testing.T) {
	t.Parallel()
	observations, counts, err := healthband.NewStore(t.TempDir()).ReadObservations(healthband.CIRunsFile)

	require.NoError(t, err)
	assert.Empty(t, observations)
	assert.Zero(t, counts)
}

// Data Contracts: the CI observation line is byte-exact, written in UTC.
func TestAppendObservations_WritesContractLinesInUTC(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := lockStore(t, dir)
	seoul := time.FixedZone("KST", 9*60*60)
	observation := ciObservation(1042, time.Date(2026, 9, 14, 19, 0, 0, 0, seoul), 1, 2)

	require.NoError(t, locked.AppendObservations(healthband.CIRunsFile, []healthband.Observation{observation}))

	assert.Equal(t, []string{contractCILine}, readLines(t, dir, healthband.CIRunsFile))
}

func TestAppendObservations_RejectsInvalidBatchWithoutWriting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := lockStore(t, dir)
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	batch := []healthband.Observation{ciObservation(1, at, 1, 1), ciObservation(2, at, 2, 1)}

	err := locked.AppendObservations(healthband.CIRunsFile, batch)

	require.ErrorIs(t, err, healthband.ErrInvalidObservation)
	_, statErr := os.Stat(metricsPath(dir, healthband.CIRunsFile))
	assert.True(t, os.IsNotExist(statErr))
}

// A crash can leave a partial last line; the next append must not glue its
// first line onto it.
func TestAppendObservations_KeepsNewLineSeparateFromPartialTail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile, `{"schema":"autopus.metric_observation.v1","seri`)
	locked := lockStore(t, dir)

	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	require.NoError(t, locked.AppendObservations(healthband.CIRunsFile, []healthband.Observation{ciObservation(7, at, 0, 1)}))

	observations, counts, err := locked.Store().ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"7"}, sampleKeys(observations))
	assert.Equal(t, 1, counts.Malformed)
}

// S4 (store part): merging is idempotent by (series, sample_key, attempt);
// only a higher attempt appends, and it supersedes in the collapsed view.
func TestMergeObservations_AppendsOnlyNewSamplesAndHigherAttempts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := lockStore(t, dir)
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	payload := []healthband.Observation{
		ciObservation(500, at, 0, 2), ciObservation(502, at, 1, 1), ciObservation(505, at, 1, 1),
	}

	first, err := locked.MergeObservations(healthband.CIRunsFile, payload)
	require.NoError(t, err)
	again, err := locked.MergeObservations(healthband.CIRunsFile, payload)
	require.NoError(t, err)
	superseding, err := locked.MergeObservations(healthband.CIRunsFile, []healthband.Observation{ciObservation(500, at, 1, 3)})
	require.NoError(t, err)
	stale, err := locked.MergeObservations(healthband.CIRunsFile, []healthband.Observation{ciObservation(500, at, 1, 1)})
	require.NoError(t, err)
	duplicateInBatch, err := locked.MergeObservations(healthband.CIRunsFile,
		[]healthband.Observation{ciObservation(506, at, 0, 1), ciObservation(506, at, 1, 1)})
	require.NoError(t, err)

	assert.Len(t, first, 3)
	assert.Empty(t, again)
	assert.Equal(t, []healthband.Observation{ciObservation(500, at, 1, 3)}, superseding)
	assert.Empty(t, stale)
	assert.Equal(t, []healthband.Observation{ciObservation(506, at, 0, 1)}, duplicateInBatch)
	assert.Len(t, readLines(t, dir, healthband.CIRunsFile), 5)
	observations, _, err := locked.Store().ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	collapsed := healthband.OrderedSeries(observations)["ci.failure_rate:CI"]
	require.Len(t, collapsed, 4)
	assert.Equal(t, ciObservation(500, at, 1, 3), collapsed[0])
}

func TestMergeObservations_RejectsInvalidCandidate(t *testing.T) {
	t.Parallel()
	locked := lockStore(t, t.TempDir())
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	appended, err := locked.MergeObservations(healthband.CIRunsFile, []healthband.Observation{ciObservation(1, at, 3, 1)})

	require.True(t, errors.Is(err, healthband.ErrInvalidObservation))
	assert.Empty(t, appended)
}
